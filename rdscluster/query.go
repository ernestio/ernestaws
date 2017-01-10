/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdscluster

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

// FindRDSClusters : Find rds clusters on aws
func FindRDSClusters(q *ernestaws.Query) error {
	svc := getRDSClient(q)

	req := &rds.DescribeDBClustersInput{
		Filters: mapFilters(q.Tags),
	}

	resp, err := svc.DescribeDBClusters(req)
	if err != nil {
		return err
	}

	for _, c := range resp.DBClusters {
		tags, err := getClusterTagDescriptions(svc, c.DBClusterIdentifier)
		if err != nil {
			return err
		}

		sg, err := getSubnetGroup(svc, c.DBSubnetGroup)
		if err != nil {
			return err
		}

		q.Results = append(q.Results, ToEvent(c, sg, tags))
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

func getClusterTagDescriptions(svc *rds.RDS, name *string) ([]*rds.Tag, error) {
	treq := &rds.ListTagsForResourceInput{
		ResourceName: name,
	}

	resp, err := svc.ListTagsForResource(treq)

	return resp.TagList, err
}

func getSubnetGroup(svc *rds.RDS, name *string) (*rds.DBSubnetGroup, error) {
	req := &rds.DescribeDBSubnetGroupsInput{
		DBSubnetGroupName: name,
	}

	resp, err := svc.DescribeDBSubnetGroups(req)

	return resp.DBSubnetGroups[0], err
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

// ToEvent converts an rds cluster object to an ernest event
func ToEvent(c *rds.DBCluster, sg *rds.DBSubnetGroup, tags []*rds.Tag) *Event {
	e := &Event{
		Name:                *c.DBClusterIdentifier,
		Engine:              *c.Engine,
		EngineVersion:       c.EngineVersion,
		Port:                c.Port,
		Endpoint:            *c.Endpoint,
		AvailabilityZones:   c.AvailabilityZones,
		DatabaseName:        c.DatabaseName,
		DatabaseUsername:    c.MasterUsername,
		BackupRetention:     c.BackupRetentionPeriod,
		BackupWindow:        c.PreferredBackupWindow,
		MaintenanceWindow:   c.PreferredMaintenanceWindow,
		ReplicationSource:   c.ReplicationSourceIdentifier,
		NetworkAWSIDs:       mapSubnetGroups(sg),
		SecurityGroupAWSIDs: mapRDSSecurityGroups(c.VpcSecurityGroups),
		Tags:                mapRDSTags(tags),
	}
	return e
}
