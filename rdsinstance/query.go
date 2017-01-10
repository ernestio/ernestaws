/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdsinstance

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

func getRDSClient(q *ernestaws.Query) *rds.RDS {
	creds, _ := credentials.NewStaticCredentials(q.AWSAccessKeyID, q.AWSSecretAccessKey, q.CryptoKey)
	return rds.New(session.New(), &aws.Config{
		Region:      aws.String(q.DatacenterRegion),
		Credentials: creds,
	})
}

func mapFilters(tags map[string]string) []*rds.Filter {
	var f []*rds.Filter

	for key, val := range tags {
		f = append(f, &rds.Filter{
			Name:   aws.String("tag:" + key),
			Values: []*string{aws.String(val)},
		})
	}

	return f
}

// FindRDSInstances : Find rds clusters on aws
func FindRDSInstances(q *ernestaws.Query) error {
	svc := getRDSClient(q)

	req := &rds.DescribeDBInstancesInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeDBInstances(req)
	if err != nil {
		return err
	}

	for _, i := range resp.DBInstances {
		tags, err := getInstanceTagDescriptions(svc, i.DBInstanceIdentifier)
		if err != nil {
			return err
		}

		q.Results = append(q.Results, ToEvent(i, tags))
	}

	return nil
}

func mapRDSTags(input []*rds.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func getInstanceTagDescriptions(svc *rds.RDS, name *string) ([]*rds.Tag, error) {
	treq := &rds.ListTagsForResourceInput{
		ResourceName: name,
	}

	resp, err := svc.ListTagsForResource(treq)

	return resp.TagList, err
}

func mapSubnetGroups(subnetgroup *rds.DBSubnetGroup) []*string {
	var sids []*string

	for _, s := range subnetgroup.Subnets {
		sids = append(sids, s.SubnetIdentifier)
	}

	return sids
}

func mapRDSSecurityGroups(sgroups []*rds.VpcSecurityGroupMembership) []*string {
	var sgs []*string

	for _, s := range sgroups {
		sgs = append(sgs, s.VpcSecurityGroupId)
	}

	return sgs
}

// ToEvent converts an rds instance object to an ernest event
func ToEvent(i *rds.DBInstance, tags []*rds.Tag) *Event {
	e := &Event{
		Name:                *i.DBClusterIdentifier,
		Endpoint:            *i.Endpoint.Address,
		Port:                i.Endpoint.Port,
		Engine:              *i.Engine,
		EngineVersion:       i.EngineVersion,
		Public:              *i.PubliclyAccessible,
		MultiAZ:             *i.MultiAZ,
		PromotionTier:       i.PromotionTier,
		AutoUpgrade:         *i.AutoMinorVersionUpgrade,
		Cluster:             i.DBClusterIdentifier,
		DatabaseName:        i.DBName,
		DatabaseUsername:    i.MasterUsername,
		StorageIops:         i.Iops,
		StorageType:         i.StorageType,
		StorageSize:         i.AllocatedStorage,
		BackupRetention:     i.BackupRetentionPeriod,
		BackupWindow:        i.PreferredBackupWindow,
		MaintenanceWindow:   i.PreferredMaintenanceWindow,
		ReplicationSource:   i.ReadReplicaSourceDBInstanceIdentifier,
		License:             i.LicenseModel,
		Timezone:            i.Timezone,
		SecurityGroupAWSIDs: mapRDSSecurityGroups(i.VpcSecurityGroups),
		NetworkAWSIDs:       mapSubnetGroups(i.DBSubnetGroup),
		AvailabilityZone:    i.AvailabilityZone,
		// FinalSnapshot: i. Cannot be inferred ....

		Tags: mapRDSTags(tags),
	}
	return e
}
