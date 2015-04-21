package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/awsutil"
	"github.com/awslabs/aws-sdk-go/service/route53"
	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/golang/glog"

	"github.com/brandnetworks/route53-registrator/healthcheck"
)

func getopt(name, def string) string {
	if env := os.Getenv(name); env != "" {
		return env
	}
	return def
}

func assert(err error) {
	if err != nil {
		glog.Error(err)
	}
}

func containerIsRunning(client *dockerapi.Client, containerName string) (running bool, err error) {
	//defaults to only the running containers
	containers, err := client.ListContainers(dockerapi.ListContainersOptions{})
	if err != nil {
		return false, err
	}
	found := false
	for _, container := range containers {
		for _, name := range container.Names {
			fmt.Println(normalizedContainerName(name), normalizedContainerName(containerName))
			if normalizedContainerName(name) == normalizedContainerName(containerName) {
				found = true
				break
			}
		}
	}
	return found, nil
}

//uses the ec2 metadata service to retrieve the public
//cname for the instance
func hostname(metadataServerAddress string) (hostname string) {
	host := []string{"http:/", metadataServerAddress, "latest", "meta-data", "public-hostname"}
	resp, err := http.Get(strings.Join(host, "/"))
	assert(err)

	defer resp.Body.Close()
	assert(err)
	body, err := ioutil.ReadAll(resp.Body)
	assert(err)
	return string(body)
}

//container names start with a /. This function removes the leading / if it exists.
func normalizedContainerName(original string) (normalized string) {
	if strings.HasPrefix(original, "/") {
		return original
	}
	return strings.Join([]string{"/", original}, "")
}

//Given a container ID and a name, assert whether the name of the container matches that of the provided name.
func isObservedContainer(client *dockerapi.Client, containerId string, targetContainerName string) (observed bool) {
	container, err := client.InspectContainer(containerId)
	assert(err)
	if container.Name == targetContainerName {
		return true
	}
	glog.Infof("Container ", containerId, " did not match name", targetContainerName)
	return false
}

//Find all resource records in a AWS Hosted Zone that match a given name.
func findMatchingResourceRecordsByName(client *route53.Route53, zone string, setName string) (recordSet []*route53.ResourceRecordSet, err error) {
	resources, err := client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneID: aws.String(zone),
	})
	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		glog.Errorf("AWS Error: \n Code: %s \n Message: %s", awserr.Code, awserr.Message)
		return nil, err
	} else if err != nil {
		// A non-service error occurred.
		return nil, err
	}
	var matching []*route53.ResourceRecordSet
	for _, route := range resources.ResourceRecordSets {
		//FQDNs have a trailing dot. Check if that which has been provided
		//matches the route, irrespective of the trailing dot
		if strings.TrimRight(*route.Name, ".") == strings.TrimRight(setName, ".") {
			matching = append(matching, route)
		}
	}
	return matching, nil
}

//Creates a ResourceRecordSet with a default TTL and Weight.
//The SetIdentifier equals the the hostname of the server.
func WeightedCNAMEForValue(cname string, value string) (resourceRecordSet *route53.ResourceRecordSet) {
	return &route53.ResourceRecordSet{
		Name: aws.String(cname),
		Type: aws.String("CNAME"),
		ResourceRecords: []*route53.ResourceRecord{
			&route53.ResourceRecord{
				Value: aws.String(value),
			},
		},
		SetIdentifier: aws.String(value),
		TTL:           aws.Long(5),
		Weight:        aws.Long(50),
	}
}

//Creates the necessary params for a ChangeResourceRecordRequest
func paramsForChangeResourceRecordRequest(client *route53.Route53, action string, zoneId string, resourceRecordSet *route53.ResourceRecordSet) route53.ChangeResourceRecordSetsInput {
	changes := []*route53.Change{&route53.Change{
		Action:            aws.String(action),
		ResourceRecordSet: resourceRecordSet,
	}}
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: changes,
		},
		HostedZoneID: aws.String(zoneId),
	}
	return *params
}

//Defines a Route53 request for a CNAME
type requestFn func(client *route53.Route53, action string, zoneId string, healthcheckId string, cname string, value string) (resp *route53.ChangeResourceRecordSetsOutput, err error)

//Executes the ChangeResourceRecordSet
func route53ChangeRequest(client *route53.Route53, action string, healthcheckId string, zoneId string, cname string, value string) (resp *route53.ChangeResourceRecordSetsOutput, err error) {
	resourceRecordSet := WeightedCNAMEForValue(cname, value)
	params := paramsForChangeResourceRecordRequest(client, action, zoneId, resourceRecordSet)
	return client.ChangeResourceRecordSets(&params)
}

//A closure that consumes a requestFn as a parameter
//and returns a requestFn that handles errors resulting
//from it's execution
func ErrorHandledRequestFn(reqFn requestFn) (wrapped requestFn) {
	return func(route53Client *route53.Route53, action string, zoneId string, healthcheckId string, cname string, value string) (resp *route53.ChangeResourceRecordSetsOutput, err error) {
		resp, err = reqFn(route53Client, action, zoneId, healthcheckId, cname, value)
		if awserr := aws.Error(err); awserr != nil {
			glog.Errorf("AWS Error: \n Code: %s \n Message: %s", awserr.Code, awserr.Message)
			return nil, err
		} else if err != nil {
			// A non-service error occurred.
			glog.Errorf("Error occured creating records: \n %s", err)
			return nil, err
		}
		glog.Infof("Response received for request: \n %s", awsutil.StringValue(resp))
		return resp, nil
	}
}

//I'm not sure Rob Pike would approve of this, but here's a sort of currying
type requestFnForZoneClient func(action string, healthcheckId string, cname string, value string) (resp *route53.ChangeResourceRecordSetsOutput, err error)

func requestFnForClientZone(client *route53.Route53, zoneId string, fn requestFn) (curried requestFnForZoneClient) {
	return func(action string, healthcheckId string, cname string, value string) (resp *route53.ChangeResourceRecordSetsOutput, err error) {
		return fn(client, action, zoneId, healthcheckId, cname, value)
	}
}

func main() {
	var containerName = flag.String("container", "docker-registry", "The container to watch")
	var metadataIP = flag.String("metadata", "169.254.169.254", "The address of the metadata service")
	var region = flag.String("region", "us-east-1", "The region for route53 records")
	var zoneId = flag.String("zone", "Z1P7DHMHEAX6O3", "The route53 hosted zone id")
	var cname = flag.String("cname", "my-test-registry.realtime.bnservers.com", "The CNAME for the record set")

	//Print some debug information
	flag.Parse()
	glog.Info(*region)
	glog.Info(*metadataIP)
	glog.Info(*cname)
	glog.Info(*zoneId)
	glog.Info(*containerName)

	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///tmp/docker.sock"))
	assert(err)
	err = docker.Ping()
	assert(err)

	//the container name we're looking for
	targetContainer := normalizedContainerName(*containerName)

	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))
	client := route53.New(nil)

	weightedCNAMEFn := ErrorHandledRequestFn(route53ChangeRequest)
	weightedRequestForClientZone := requestFnForClientZone(client, *zoneId, weightedCNAMEFn)

	//check if the named container is alive on the host
	running, err := containerIsRunning(docker, *containerName)
	if err != nil {
		glog.Errorf("Error checking for existing container: %s", err)
	}

	//check there is a healthcheck that exists for this hostname
	exists, healthCheckFqdn, err := healthcheck.HealthCheckForFQDNPort(client, hostname(*metadataIP), aws.Long(1000))
	if err != nil {
		glog.Errorf("Error checking for existing health check: %s", err)
	}

	//create one if there isn't
	if !exists {
		glog.Infof("No healthcheck found for endpoint. Creating.")
		healthCheckFqdn, err = healthcheck.CreateHealthCheck(client, aws.String(hostname(*metadataIP)), 1000, aws.String("/v1/_ping"), aws.String(hostname(*metadataIP)))
		if err != nil {
			glog.Errorf("Error creating health check: %s", err)
		}
	} else {
		glog.Infof("Found a matching health check for FQDN %s and port %d", hostname(*metadataIP), 1000)
	}

	//if the container is running, then check if there is an existing record pointing
	//to this host. If there is not, then create one.
	if running {
		matchingResourceRecords, err := findMatchingResourceRecordsByName(client, *zoneId, *cname)
		exists := false
		for _, recordSet := range matchingResourceRecords {
			for _, record := range recordSet.ResourceRecords {
				if *record.Value == hostname(*metadataIP) {
					glog.Infof("Found record with Name %s and value %s", *cname, hostname(*metadataIP))
					exists = true
				}
			}
		}
		if !exists {
			glog.Infof("No record exists with Name %s and value %s. Creating.", *cname, hostname(*metadataIP))
			weightedRequestForClientZone("CREATE", healthCheckFqdn.HealthCheckID, *cname, hostname(*metadataIP))
		}
		if err != nil {
			glog.Errorf("Error searching for exisiting records:", err)
		}
	}

	glog.Infof("Listening for Docker events ...")

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			glog.Infof("Event: container %s started", msg.ID)
			if isObservedContainer(docker, msg.ID, targetContainer) {
				weightedRequestForClientZone("CREATE", healthCheckFqdn.HealthCheckID, *cname, hostname(*metadataIP))
			}
		case "die":
			glog.Infof("Event: container %s died.", msg.ID)
			if isObservedContainer(docker, msg.ID, targetContainer) {
				weightedRequestForClientZone("DELETE", healthCheckFqdn.HealthCheckID, *cname, hostname(*metadataIP))
				//delete the health check, too
			}
		case "default":
			glog.Infof("Event: container %s ignoring", msg.ID)
		}
	}

	quit := make(chan struct{})
	close(quit)
}
