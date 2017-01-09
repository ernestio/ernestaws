/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package nat

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

func getEC2Client(q *ernestaws.Query) *ec2.EC2 {
	creds, _ := credentials.NewStaticCredentials(q.AWSAccessKeyID, q.AWSSecretAccessKey, q.CryptoKey)
	return ec2.New(session.New(), &aws.Config{
		Region:      aws.String(q.DatacenterRegion),
		Credentials: creds,
	})
}

func mapFilters(tags map[string]string) []*ec2.Filter {
	var f []*ec2.Filter

	for key, val := range tags {
		f = append(f, &ec2.Filter{
			Name:   aws.String("tag:" + key),
			Values: []*string{aws.String(val)},
		})
	}

	return f
}

// FindNatGateways : Find nat gateways on aws
func FindNatGateways(q *ernestaws.Query) error {
	svc := getEC2Client(q)

	req := &ec2.DescribeNatGatewaysInput{
		Filter: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeNatGateways(req)
	if err != nil {
		return err
	}

	for _, ng := range resp.NatGateways {
		tags, err := getGatewayTagDescriptions(svc, ng.NatGatewayId)
		if err != nil {
			return err
		}

		q.Results = append(q.Results, ToEvent(ng, tags))
	}

	return nil
}

func mapEC2Tags(input []*ec2.TagDescription) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func getGatewayTagDescriptions(svc *ec2.EC2, id *string) ([]*ec2.TagDescription, error) {
	var treq *ec2.DescribeTagsInput

	treq.Filters = append(treq.Filters, &ec2.Filter{
		Name:   aws.String("resource-id"),
		Values: []*string{id},
	})

	resp, err := svc.DescribeTags(treq)

	return resp.Tags, err
}

func getPublicAllocation(addresses []*ec2.NatGatewayAddress) (string, string) {
	for _, a := range addresses {
		if a.PublicIp != nil {
			return *a.AllocationId, *a.PublicIp
		}
	}
	return "", ""
}

// ToEvent converts an ec2 nat gateway object to an ernest event
func ToEvent(ng *ec2.NatGateway, tags []*ec2.TagDescription) *Event {
	id, ip := getPublicAllocation(ng.NatGatewayAddresses)

	e := &Event{
		VPCID:                  *ng.VpcId,
		NatGatewayAWSID:        *ng.NatGatewayId,
		NetworkAWSID:           *ng.SubnetId,
		NatGatewayAllocationID: id,
		NatGatewayAllocationIP: ip,
		//RoutedNetworksAWSIDs
		//InternetGatewayID
		Tags: mapEC2Tags(tags),
	}
	return e
}
