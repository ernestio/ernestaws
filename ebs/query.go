/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ebs

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

// FindEBSs : Find ebs on aws
func FindEBSs(q *ernestaws.Query) error {
	svc := getEC2Client(q)

	req := &ec2.DescribeVolumesInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeVolumes(req)
	if err != nil {
		return err
	}

	for _, v := range resp.Volumes {
		q.Results = append(q.Results, ToEvent(v))
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

// ToEvent converts an ec2 instance object to an ernest event
func ToEvent(v *ec2.Volume) *Event {
	return &Event{
		VolumeAWSID:      *v.VolumeId,
		AvailabilityZone: *v.AvailabilityZone,
		VolumeType:       *v.VolumeType,
		Size:             v.Size,
		Iops:             v.Iops,
		Encrypted:        *v.Encrypted,
		EncryptionKeyID:  v.KmsKeyId,
		Tags:             mapEC2Tags(v.Tags),
	}
}
