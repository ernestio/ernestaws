/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ernestaws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/ernestio/ernestaws/credentials"
	"github.com/ernestio/ernestaws/instance"
	"github.com/ernestio/ernestaws/network"
	"github.com/ernestio/ernestaws/vpc"
)

// Query ....
type Query struct {
	UUID               string            `json:"_uuid"`
	BatchID            string            `json:"_batch_id"`
	ProviderType       string            `json:"_type"`
	AWSAccessKeyID     string            `json:"aws_access_key_id"`
	AWSSecretAccessKey string            `json:"aws_secret_access_key"`
	DatacenterRegion   string            `json:"datacenter_region"`
	Tags               map[string]string `json:"tags"`

	Results   []interface{} `json:"results"`
	CryptoKey string        `json:"-"`
}

func (q *Query) getEC2Client() *ec2.EC2 {
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

// Find : Find vpcs on aws
func (q Query) findVPCs() error {
	svc := q.getEC2Client()

	req := &ec2.DescribeVpcsInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeVpcs(req)
	if err != nil {
		return err
	}

	for _, v := range resp.Vpcs {
		q.Results = append(q.Results, vpc.ToEvent(v))
	}

	return nil
}

// Find : Find networks on aws
func (q Query) findNetworks() error {
	svc := q.getEC2Client()

	req := &ec2.DescribeSubnetsInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeSubnets(req)
	if err != nil {
		return err
	}

	for _, n := range resp.Subnets {
		q.Results = append(q.Results, network.ToEvent(n))
	}

	return nil
}

// Find : Find instances on aws
func (q Query) findInstances() error {
	svc := q.getEC2Client()

	req := &ec2.DescribeInstancesInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeInstances(req)
	if err != nil {
		return err
	}

	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			q.Results = append(q.Results, instance.ToEvent(i))
		}
	}

	return nil
}

// Find : Find volumes on aws
func (q Query) findInstances() error {
	svc := q.getEC2Client()

	req := &ec2.DescribeVolumesInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeVolumes(req)
	if err != nil {
		return err
	}

	for _, v := range resp.Volumes {
		q.Results = append(q.Results, ebs.ToEvent(v))
	}

	return nil

}
