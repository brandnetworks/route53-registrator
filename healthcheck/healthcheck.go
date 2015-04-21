package healthcheck

import (
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"github.com/nu7hatch/gouuid"
	"strings"
	"time"
)

type HealthCheckFQDN struct {
	Fqdn          string
	HealthCheckID string
}

//Creates the Config required for a health check, including some
//default values
func createHealthCheckInput(uniqueId *string, fqdn *string, port int64, resourcepath *string) (input *route53.CreateHealthCheckInput, err error) {
	config := route53.HealthCheckConfig{
		FailureThreshold:         aws.Long(1),
		FullyQualifiedDomainName: fqdn,
		RequestInterval:          aws.Long(10),
		ResourcePath:             resourcepath,
		Type:                     aws.String("HTTP"),
		Port:                     aws.Long(port),
	}

	//caller references have to be unique on every request
	callerrerference, err := uuid.NewV4()
	if err != nil {
		return nil, nil
	}
	return &route53.CreateHealthCheckInput{
		CallerReference:   aws.String(callerrerference.String()),
		HealthCheckConfig: &config,
	}, nil
}

//given a domain name, check if it there is a healthcheck for it.
func HealthCheckForFQDNPort(client *route53.Route53, fqdn string, port *int64) (exists bool, check HealthCheckFQDN, err error) {
	resp, err := getHealthChecks(client)

	//check if any of them contain the healthcheck.
	//assumption is that there will be a unique health check
	//per fqdn/port
	for _, healthcheck := range resp.HealthChecks {
		config := healthcheck.HealthCheckConfig
		if *config.FullyQualifiedDomainName == fqdn {
			if *config.Port == *port {
				return true, HealthCheckFQDN{
					Fqdn:          *healthcheck.HealthCheckConfig.FullyQualifiedDomainName,
					HealthCheckID: *healthcheck.ID,
				}, nil
			}
		} else {
			glog.Infof("No string match")
		}
	}
	return false, HealthCheckFQDN{}, nil
}

//Create a Health Check.
func CreateHealthCheck(client *route53.Route53, hostname *string, port int64, resourcePath *string, fqdn *string) (check HealthCheckFQDN, err error) {
	input, err := createHealthCheckInput(hostname, fqdn, port, resourcePath)
	if err != nil {
		return HealthCheckFQDN{}, err
	}
	resp, err := client.CreateHealthCheck(input)

	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		glog.Errorf("AWS Error: \n Code: %s \n Message: %s", awserr.Code, awserr.Message)
		panic(err)
	} else if err != nil {
		// A non-service error occurred.
		glog.Errorf("Error occured creating records: \n %s", err)
		panic(err)
	}

	return HealthCheckFQDN{
		Fqdn:          *fqdn,
		HealthCheckID: *resp.HealthCheck.ID,
	}, nil
}

//Given a set of resource record sets, find all the health checks that haven't
//passed recently. If they're older than 30 seconds, then delete them
func FindFailingHealthChecks(client *route53.Route53, identifier string) (checks []*route53.HealthCheck, err error) {
	resp, _ := getHealthChecks(client)
	var failing []*route53.HealthCheck
	for _, check := range resp.HealthChecks {
		//get the tags
		listTagsResp, _ := getTagsForHealthCheck(client, check.ID)
		//find the tag with the expected key, and check it's value
		for _, tag := range listTagsResp.ResourceTagSet.Tags {
			if *tag.Key == "route53-registrator" {
				//if the healthcheck is managed by *this* registrator (identified by a tag on the health check), then add it to the list of the failing health checks
				if *tag.Value == identifier {
					resp, err := client.GetHealthCheckStatus(&route53.GetHealthCheckStatusInput{
						HealthCheckID: check.ID,
					})
					if awserr := aws.Error(err); awserr != nil {
						// A service error occurred.
						glog.Errorf("AWS Error: \n Code: %s \n Message: %s", awserr.Code, awserr.Message)
						panic(err)
					} else if err != nil {
						// A non-service error occurred.
						glog.Errorf("Error occured finding healthcheck status: \n %s", err)
						panic(err)
					}
					for _, observation := range resp.HealthCheckObservations {
						if strings.Contains(*observation.StatusReport.Status, "Failure") {
							//only return those that have been unhealthy for 30 seconds
							if observation.StatusReport.CheckedTime.Before(time.Now().Add(time.Duration(30) * time.Second)) {
								glog.Infof("Healthcheck with matching tag failed over 30 seconds ago: ", *observation.StatusReport.Status)
								failing = append(failing, check)
							}
						}
					}
				}
			}
		}
	}
	return failing, nil
}

func getHealthChecks(client *route53.Route53) (out route53.ListHealthChecksOutput, err error) {
	resp, err := client.ListHealthChecks(&route53.ListHealthChecksInput{})
	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		glog.Errorf("AWS Error: \n Code: %s \n Message: %s", awserr.Code, awserr.Message)
		panic(err)
	} else if err != nil {
		// A non-service error occurred.
		glog.Errorf("Error occured creating records: \n %s", err)
		panic(err)
	}
	return *resp, nil
}

func getTagsForHealthCheck(client *route53.Route53, healthcheckId *string) (out route53.ListTagsForResourceOutput, err error) {
	resp, err := client.ListTagsForResource(&route53.ListTagsForResourceInput{
		ResourceID:   healthcheckId,
		ResourceType: aws.String("healthcheck"),
	})
	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		glog.Errorf("AWS Error: \n Code: %s \n Message: %s", awserr.Code, awserr.Message)
		panic(err)
	} else if err != nil {
		// A non-service error occurred.
		glog.Errorf("Error occured finding tags: \n %s", err)
		panic(err)
	}
	return *resp, nil
}
