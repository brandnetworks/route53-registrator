package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/creack/goamz/route53"
	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/mitchellh/goamz/aws"
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

func createResourceRecords(action string, hostname string, name string, recordType string) (req *route53.ChangeResourceRecordSetsRequest) {
	return &route53.ChangeResourceRecordSetsRequest{
		Comment: "Test",
		Changes: []route53.Change{
			route53.Change{
				Action: action,
				Record: route53.ResourceRecordSet{
					Name:          name,
					Type:          recordType,
					TTL:           5,
					Records:       []string{hostname},
					Weight:        50,
					SetIdentifier: hostname,
				},
			},
		},
	}
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

	//we're going for the either env or instance profile here
	auth, err := aws.GetAuth("", "")
	assert(err)
	client := route53.New(auth, aws.Regions[*region])

	log.Println("Listening for Docker events ...")

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			log.Println("docker start")
			if isObservedContainer(docker, msg.ID, targetContainer) {
				_, err := client.ChangeResourceRecordSets(*zoneId, createResourceRecords("CREATE", hostname(*metadataIP), *cname, "CNAME"))
				assert(err)
			}
		case "die":
			log.Println("docker die")
			if isObservedContainer(docker, msg.ID, targetContainer) {
				_, err := client.ChangeResourceRecordSets(*zoneId, createResourceRecords("DELETE", hostname(*metadataIP), *cname, "CNAME"))
				assert(err)
			}
		case "default":
			log.Println(msg)
		}
	}

	quit := make(chan struct{})
	close(quit)
	log.Println("bye")
}
