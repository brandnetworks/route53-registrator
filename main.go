package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/awsutil"
	"github.com/awslabs/aws-sdk-go/service/route53"
	dockerapi "github.com/fsouza/go-dockerclient"
)

func getopt(name, def string) string {
	if env := os.Getenv(name); env != "" {
		return env
	}
	return def
}

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
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

//container names start with a /
func normalizedContainerName(original string) (normalized string) {
	if strings.HasPrefix("/", original) {
		return original
	}
	return strings.Join([]string{"/", original}, "")
}

func isObservedContainer(client *dockerapi.Client, containerId string, targetContainerName string) (observed bool) {
	container, err := client.InspectContainer(containerId)
	assert(err)
	if container.Name == targetContainerName {
		return true
	}
	log.Println("no match")
	return false
}

func findMatchingResourceRecords(client *route53.Route53, zone string, setName string) (recordSet []*route53.ResourceRecordSet, err error) {
	resources, err := client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneID: aws.String(zone),
	})
	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		fmt.Println("Error:", awserr.Code, awserr.Message)
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
			fmt.Println("Found existing ResourceRecord Set with name ", strings.TrimRight(setName, "."))
			fmt.Println(awsutil.StringValue(route))
			matching = append(matching, route)
		}
	}
	return matching, nil
}

func createResourceRecordSet(cname string, value string) (resourceRecordSet *route53.ResourceRecordSet) {
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

func main() {
	var containerName = flag.String("container", "docker-registry", "The container to watch")
	var metadataIP = flag.String("metadata", "169.254.169.254", "The address of the metadata service")
	var region = flag.String("region", "us-east-1", "The region for route53 records")
	var zoneId = flag.String("zone", "Z1P7DHMHEAX6O3", "The route53 hosted zone id")
	var cname = flag.String("cname", "my-test-registry.realtime.bnservers.com", "The CNAME for the record set")

	flag.Parse()
	log.Println(*region)
	log.Println(*metadataIP)
	log.Println(*cname)
	log.Println(*zoneId)
	log.Println(*containerName)

	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///tmp/docker.sock"))
	assert(err)
	err = docker.Ping()
	assert(err)

	//the container name we're looking for
	targetContainer := normalizedContainerName(*containerName)

	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))

	client := route53.New(nil)

	matchingResourceRecords, err := findMatchingResourceRecords(client, *zoneId, *cname)
	if err != nil {
		log.Fatalf("Failed:", err)
	}
	fmt.Printf("Found %d existing records with a matching name. Destroying. \n", len(matchingResourceRecords))
	if matchingResourceRecords != nil {
		var changes []*route53.Change
		for _, set := range matchingResourceRecords {
			changes = append(changes, &route53.Change{
				Action:            aws.String("DELETE"),
				ResourceRecordSet: set,
			})
		}
		params := &route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: changes,
			},
			HostedZoneID: aws.String(*zoneId),
		}
		resp, err := client.ChangeResourceRecordSets(params)
		if awserr := aws.Error(err); awserr != nil {
			fmt.Println("Error:", awserr.Code, awserr.Message)
		} else if err != nil {
			fmt.Println("Error removing existing records: ", err)
			panic(err)
		}
		fmt.Println(awsutil.StringValue(resp))
	}

	log.Println("Listening for Docker events ...")

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			log.Println("docker start")
			if isObservedContainer(docker, msg.ID, targetContainer) {
				var changes []*route53.Change
				changes = append(changes, &route53.Change{
					Action:            aws.String("CREATE"),
					ResourceRecordSet: createResourceRecordSet(*cname, hostname(*metadataIP)),
				})
				params := &route53.ChangeResourceRecordSetsInput{
					ChangeBatch: &route53.ChangeBatch{
						Changes: changes,
					},
					HostedZoneID: aws.String(*zoneId),
				}
				resp, err := client.ChangeResourceRecordSets(params)

				if awserr := aws.Error(err); awserr != nil {
					fmt.Println("Error:", awserr.Code, awserr.Message)
				} else if err != nil {
					// A non-service error occurred.
					panic(err)
				}
				fmt.Println(awsutil.StringValue(resp))
			}
		case "die":
			log.Println("docker die")
			if isObservedContainer(docker, msg.ID, targetContainer) {
				var changes []*route53.Change
				changes = append(changes, &route53.Change{
					Action:            aws.String("DELETE"),
					ResourceRecordSet: createResourceRecordSet(*cname, hostname(*metadataIP)),
				})
				params := &route53.ChangeResourceRecordSetsInput{
					ChangeBatch: &route53.ChangeBatch{
						Changes: changes,
					},
					HostedZoneID: aws.String(*zoneId),
				}
				resp, err := client.ChangeResourceRecordSets(params)

				if awserr := aws.Error(err); awserr != nil {
					fmt.Println("Error:", awserr.Code, awserr.Message)
				} else if err != nil {
					// A non-service error occurred.
					panic(err)
				}
				// Pretty-print the response data.
				fmt.Println(awsutil.StringValue(resp))
			}
		case "default":
			log.Println(msg)
		}
	}

	quit := make(chan struct{})
	close(quit)
	log.Println("bye")
}
