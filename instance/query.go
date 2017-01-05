/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package instance

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

// FindInstances : Find instances on aws
func FindInstances(q *ernestaws.Query) error {
	svc := getEC2Client(q)

	req := &ec2.DescribeInstancesInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeInstances(req)
	if err != nil {
		return err
	}

	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			q.Results = append(q.Results, ToEvent(i))
		}
	}

	return nil
}

func mapAWSSecurityGroupIDs(gi []*ec2.GroupIdentifier) []string {
	var sgs []string

	for _, sg := range gi {
		sgs = append(sgs, *sg.GroupId)
	}

	return sgs
}

func mapAWSVolumes(vs []*ec2.InstanceBlockDeviceMapping) []Volume {
	var vols []Volume

	for _, v := range vs {
		vols = append(vols, Volume{
			Device:      *v.DeviceName,
			VolumeAWSID: *v.Ebs.VolumeId,
		})
	}

	return vols
}

// ToEvent converts an ec2 instance object to an ernest event
func ToEvent(i *ec2.Instance) *Event {
	return &Event{
		NetworkAWSID:        *i.SubnetId,
		SecurityGroupAWSIDs: mapAWSSecurityGroupIDs(i.SecurityGroups),
		InstanceAWSID:       *i.InstanceId,
		InstanceType:        *i.InstanceType,
		Image:               *i.ImageId,
		IP:                  *i.PrivateIpAddress,
		KeyPair:             *i.KeyName,
		PublicIP:            *i.PublicIpAddress,
		Volumes:             mapAWSVolumes(i.BlockDeviceMappings),
		Tags:                mapEC2Tags(i.Tags),
	}
}
