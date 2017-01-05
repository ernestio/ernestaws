/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package network

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

// FindNetworks : Find networks on aws
func FindNetworks(q *ernestaws.Query) error {
	svc := getEC2Client(q)

	req := &ec2.DescribeSubnetsInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeSubnets(req)
	if err != nil {
		return err
	}

	for _, n := range resp.Subnets {
		q.Results = append(q.Results, ToEvent(n))
	}

	return nil
}

func mapEC2Tags(input []*ec2.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

// ToEvent converts an ec2 subnet object to an ernest event
func ToEvent(n *ec2.Subnet) *Event {
	return &Event{
		VPCID:            *n.VpcId,
		NetworkAWSID:     *n.SubnetId,
		Subnet:           *n.CidrBlock,
		AvailabilityZone: *n.AvailabilityZone,
		Tags:             mapEC2Tags(n.Tags),
	}
}
