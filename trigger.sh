#!/usr/bin/env bash

image=$1
region=$2
hosted_zone=$3
domain=$4
change_batch=$(pwd)/change_batch.json
public_hostname=$(curl http://169.254.169.254/latest/meta-data/public-hostname)

while read line; do
	if [[ "$line" == *"start"* ]]; then
		cat >$change_batch <<EOL
		{
			"Changes": [{
				"Action": "UPSERT",
				"ResourceRecordSet": {
					"Name": "${domain}.",
					"Weight": 50,
					"Type": "CNAME",
					"ResourceRecords": [{
						"Value": "${public_hostname}"
					}],
					"TTL": 5,
					"SetIdentifier": "${domain}"
				}
			}]
		}
EOL
		echo "$image started"
		aws route53 change-resource-record-sets --region $region --hosted-zone-id $hosted_zone --change-batch file://$change_batch
	elif [[ "$line" == *"die"* ]]; then
		cat >$change_batch <<EOL2
		{
			"Changes": [{
				"Action": "DELETE",
				"ResourceRecordSet": {
					"Name": "${domain}.",
					"Weight": 50,
					"Type": "CNAME",
					"TTL": 5,
					"ResourceRecords": [{
						"Value": "${public_hostname}"
					}],
					"SetIdentifier": "${domain}"
				}
			}]
		}
EOL2
		echo "$image stopped"
		aws route53 change-resource-record-sets --region $region --hosted-zone-id $hosted_zone --change-batch file://$change_batch
	fi
done

