/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdscluster

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/ernestio/ernestaws/credentials"
)

// Collection ....
type Collection struct {
	ProviderType       string            `json:"_provider"`
	ComponentType      string            `json:"_component"`
	ComponentID        string            `json:"_component_id"`
	State              string            `json:"_state"`
	Action             string            `json:"_action"`
	Service            string            `json:"service"`
	AWSAccessKeyID     string            `json:"aws_access_key_id"`
	AWSSecretAccessKey string            `json:"aws_secret_access_key"`
	DatacenterRegion   string            `json:"datacenter_region"`
	Tags               map[string]string `json:"tags"`
	Results            []interface{}     `json:"components"`
	ErrorMessage       string            `json:"error,omitempty"`
	Subject            string            `json:"-"`
	Body               []byte            `json:"-"`
	CryptoKey          string            `json:"-"`
}

// GetBody : Gets the body for this event
func (col *Collection) GetBody() []byte {
	var err error
	if col.Body, err = json.Marshal(col); err != nil {
		log.Println(err.Error())
	}
	return col.Body
}

// GetSubject : Gets the subject for this event
func (col *Collection) GetSubject() string {
	return col.Subject
}

// Process : starts processing the current message
func (col *Collection) Process() (err error) {
	if err := json.Unmarshal(col.Body, &col); err != nil {
		col.Error(err)
		return err
	}

	if err := col.Validate(); err != nil {
		col.Error(err)
		return err
	}

	return nil
}

// Error : Will respond the current event with an error
func (col *Collection) Error(err error) {
	log.Printf("Error: %s", err.Error())
	col.ErrorMessage = err.Error()
	col.State = "errored"

	col.Body, err = json.Marshal(col)
}

// Complete : sets the state of the event to completed
func (col *Collection) Complete() {
	col.State = "completed"
}

// Validate checks if all criteria are met
func (col *Collection) Validate() error {
	if col.AWSAccessKeyID == "" || col.AWSSecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	return nil
}

// Get : Gets a object on aws
func (col *Collection) Get() error {
	return errors.New(col.Subject + " not supported")
}

// Create : Creates an object on aws
func (col *Collection) Create() error {
	return errors.New(col.Subject + " not supported")
}

// Update : Updates an object on aws
func (col *Collection) Update() error {
	return errors.New(col.Subject + " not supported")
}

// Delete : Delete an object on aws
func (col *Collection) Delete() error {
	return errors.New(col.Subject + " not supported")
}

// Find : Find rds clusters on aws
func (col *Collection) Find() error {
	svc := col.getRDSClient()

	resp, err := svc.DescribeDBClusters(nil)
	if err != nil {
		return err
	}

	for _, c := range resp.DBClusters {
		tags, err := getClusterTagDescriptions(svc, c.DBClusterArn)
		if err != nil {
			return err
		}

		sg, err := getSubnetGroup(svc, c.DBSubnetGroup)
		if err != nil {
			return err
		}

		e := toEvent(c, sg, tags)

		if tagsMatch(col.Tags, e.Tags) {
			col.Results = append(col.Results)
		}
	}

	return nil
}

func (col *Collection) getRDSClient() *rds.RDS {
	creds, _ := credentials.NewStaticCredentials(col.AWSAccessKeyID, col.AWSSecretAccessKey, col.CryptoKey)
	return rds.New(session.New(), &aws.Config{
		Region:      aws.String(col.DatacenterRegion),
		Credentials: creds,
	})
}

func tagsMatch(qt, rt map[string]string) bool {
	for k, v := range qt {
		if rt[k] != v {
			return false
		}
	}

	return true
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
func toEvent(c *rds.DBCluster, sg *rds.DBSubnetGroup, tags []*rds.Tag) *Event {
	e := &Event{
		ProviderType:        "aws",
		ComponentType:       "rds_cluster",
		ComponentID:         "rds_cluster::" + *c.DBClusterIdentifier,
		ARN:                 c.DBClusterArn,
		Name:                c.DBClusterIdentifier,
		Engine:              c.Engine,
		EngineVersion:       c.EngineVersion,
		Port:                c.Port,
		Endpoint:            c.Endpoint,
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
