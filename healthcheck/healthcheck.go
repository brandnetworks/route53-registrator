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
func HealthCheckForFQDNPort(client *route53.Route53, fqdn string, port int64) (exists bool, check HealthCheckFQDN, err error) {
	resp, err := getHealthChecks(client)

	//check if any of them contain the healthcheck.
	//assumption is that there will be a unique health check
	//per fqdn/port
	for _, healthcheck := range resp.HealthChecks {
		config := healthcheck.HealthCheckConfig
		if *config.FullyQualifiedDomainName == fqdn {
			if *config.Port == port {
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

func CreateHealthCheckIfMissing(client *route53.Route53, fqdn string, port int64, endpoint string) (id string, err error) {
	exists, healthCheckFqdn, err := HealthCheckForFQDNPort(client, fqdn, port)
	if err != nil {
		glog.Errorf("Error checking for existing health check: %s", err)
	}
	if !exists {
		glog.Infof("No healthcheck found for endpoint. Creating.")
		healthCheckFqdn, err = CreateHealthCheck(client, aws.String(fqdn), port, aws.String(endpoint), aws.String(fqdn))
		if err != nil {
			glog.Errorf("Error creating health check: %s", err)
			return "", err
		}
		return healthCheckFqdn.HealthCheckID, nil
	}
	glog.Infof("Found a matching health check for FQDN %s and port %v", fqdn, port)
	return healthCheckFqdn.HealthCheckID, nil
}
