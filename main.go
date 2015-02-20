package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/route53"
)

var containerName = flag.String("container_name", "docker-registry", "The container to watch")
var metadataIP = flag.String("metadata-address", "169.254.169.254", "The address of the metadata service")
var region = flag.String("region", "us-east-1", "The region for route53 records")

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

func hostname() (hostname string) {
	host := []string{"http:/", *metadataIP, "latest", "meta-data", "public-hostname"}
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

func isObservedContainer(client *dockerapi.Client, containerId string) (observed bool) {
	container, err := client.InspectContainer(containerId)
	log.Println(container.Name)
	log.Println(normalizedContainerName(*containerName))
	assert(err)
	if container.Name == normalizedContainerName(*containerName) {
		log.Println("Matches!")
		return true
	}
	return false
}

func main() {
	flag.Parse()
	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///tmp/docker.sock"))
	assert(err)
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
			if isObservedContainer(docker, msg.ID) {
				_, err := client.ChangeResourceRecordSets("Z1P7DHMHEAX6O3", createResourceRecords("CREATE", hostname(), "my-test-set.realtime.bnservers.com", "CNAME"))
				assert(err)
			}
		case "stop":
			log.Println("docker stop")
			if isObservedContainer(docker, msg.ID) {
				_, err := client.ChangeResourceRecordSets("Z1P7DHMHEAX6O3", createResourceRecords("DELETE", hostname(), "my-test-set.realtime.bnservers.com", "CNAME"))
				assert(err)
			}
		case "die":
			log.Println("docker die")
			if isObservedContainer(docker, msg.ID) {
				_, err := client.ChangeResourceRecordSets("Z1P7DHMHEAX6O3", createResourceRecords("DELETE", hostname(), "my-test-set.realtime.bnservers.com", "CNAME"))
				assert(err)
			}
		}
	}

	quit := make(chan struct{})
	close(quit)
	log.Println("bye")
}
