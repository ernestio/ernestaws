/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdscluster

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

var (
	// ErrDatacenterIDInvalid ...
	ErrDatacenterIDInvalid = errors.New("Datacenter VPC ID invalid")
	// ErrDatacenterRegionInvalid ...
	ErrDatacenterRegionInvalid = errors.New("Datacenter Region invalid")
	// ErrDatacenterCredentialsInvalid ...
	ErrDatacenterCredentialsInvalid = errors.New("Datacenter credentials invalid")
	// ErrRDSClusterNameInvalid ...
	ErrRDSClusterNameInvalid = errors.New("RDS cluster name invalid")
	// ErrRDSClusterEngineTypeInvalid ...
	ErrRDSClusterEngineTypeInvalid = errors.New("RDS cluster engine invalid")
)

// Event stores the network data
type Event struct {
	ProviderType        string            `json:"_provider"`
	ComponentType       string            `json:"_component"`
	ComponentID         string            `json:"_component_id"`
	State               string            `json:"_state"`
	Action              string            `json:"_action"`
	ARN                 *string           `json:"arn"`
	Name                *string           `json:"name"`
	Engine              *string           `json:"engine"`
	EngineVersion       *string           `json:"engine_version,omitempty"`
	Port                *int64            `json:"port,omitempty"`
	Endpoint            *string           `json:"endpoint,omitempty"`
	AvailabilityZones   []*string         `json:"availability_zones"`
	SecurityGroups      []string          `json:"security_groups"`
	SecurityGroupAWSIDs []*string         `json:"security_group_aws_ids"`
	Networks            []string          `json:"networks"`
	NetworkAWSIDs       []*string         `json:"network_aws_ids"`
	DatabaseName        *string           `json:"database_name,omitempty"`
	DatabaseUsername    *string           `json:"database_username,omitempty"`
	DatabasePassword    *string           `json:"database_password,omitempty"`
	BackupRetention     *int64            `json:"backup_retention,omitempty"`
	BackupWindow        *string           `json:"backup_window,omitempty"`
	MaintenanceWindow   *string           `json:"maintenance_window,omitempty"`
	ReplicationSource   *string           `json:"replication_source,omitempty"`
	FinalSnapshot       *bool             `json:"final_snapshot"`
	Tags                map[string]string `json:"tags"`
	DatacenterType      string            `json:"datacenter_type"`
	DatacenterName      string            `json:"datacenter_name"`
	DatacenterRegion    string            `json:"datacenter_region"`
	AccessKeyID         string            `json:"aws_access_key_id"`
	SecretAccessKey     string            `json:"aws_secret_access_key"`
	Service             string            `json:"service"`
	ErrorMessage        string            `json:"error,omitempty"`
	Subject             string            `json:"-"`
	Body                []byte            `json:"-"`
	CryptoKey           string            `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	if strings.Split(subject, ".")[1] == "find" {
		return &Collection{Subject: subject, Body: body, CryptoKey: cryptoKey}
	}

	return &Event{Subject: subject, Body: body, CryptoKey: cryptoKey}
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AccessKeyID == "" || ev.SecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Name == nil {
		return ErrRDSClusterNameInvalid
	}

	if ev.Engine == nil {
		return ErrRDSClusterEngineTypeInvalid
	}

	return nil
}

// Process : starts processing the current message
func (ev *Event) Process() (err error) {
	if err := json.Unmarshal(ev.Body, &ev); err != nil {
		ev.Error(err)
		return err
	}

	if err := ev.Validate(); err != nil {
		ev.Error(err)
		return err
	}

	return nil
}

// Error : Will respond the current event with an error
func (ev *Event) Error(err error) {
	log.Printf("Error: %s", err.Error())
	ev.ErrorMessage = err.Error()

	ev.Body, err = json.Marshal(ev)
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	svc := ev.getRDSClient()

	subnetGroup, err := createSubnetGroup(ev)
	if err != nil {
		return err
	}

	req := &rds.CreateDBClusterInput{
		DBClusterIdentifier:         ev.Name,
		Engine:                      ev.Engine,
		EngineVersion:               ev.EngineVersion,
		Port:                        ev.Port,
		AvailabilityZones:           ev.AvailabilityZones,
		DatabaseName:                ev.DatabaseName,
		MasterUsername:              ev.DatabaseUsername,
		MasterUserPassword:          ev.DatabasePassword,
		VpcSecurityGroupIds:         ev.SecurityGroupAWSIDs,
		DBSubnetGroupName:           subnetGroup,
		BackupRetentionPeriod:       ev.BackupRetention,
		PreferredBackupWindow:       ev.BackupWindow,
		PreferredMaintenanceWindow:  ev.MaintenanceWindow,
		ReplicationSourceIdentifier: ev.ReplicationSource,
	}

	resp, err := svc.CreateDBCluster(req)
	if err != nil {
		return err
	}

	ev.ARN = resp.DBCluster.DBClusterArn
	ev.Endpoint = resp.DBCluster.Endpoint

	return ev.setTags()
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	svc := ev.getRDSClient()

	_, err := updateSubnetGroup(ev)
	if err != nil {
		return err
	}

	req := &rds.ModifyDBClusterInput{
		DBClusterIdentifier:        ev.Name,
		Port:                       ev.Port,
		MasterUserPassword:         ev.DatabasePassword,
		BackupRetentionPeriod:      ev.BackupRetention,
		PreferredBackupWindow:      ev.BackupWindow,
		PreferredMaintenanceWindow: ev.MaintenanceWindow,
		VpcSecurityGroupIds:        ev.SecurityGroupAWSIDs,
		ApplyImmediately:           aws.Bool(true),
	}

	_, err = svc.ModifyDBCluster(req)
	if err != nil {
		return err
	}

	return ev.setTags()
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBClusterInput{
		DBClusterIdentifier: ev.Name,
	}

	if *ev.FinalSnapshot {
		req.FinalDBSnapshotIdentifier = aws.String(*ev.Name + "-Final-Snapshot")
	} else {
		req.SkipFinalSnapshot = aws.Bool(true)
	}

	_, err := svc.DeleteDBCluster(req)
	if err != nil {
		return err
	}

	waitUntilClusterDeleted(ev)

	return deleteSubnetGroup(ev)
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

// GetBody : Gets the body for this event
func (ev *Event) GetBody() []byte {
	var err error
	if ev.Body, err = json.Marshal(ev); err != nil {
		log.Println(err.Error())
	}
	return ev.Body
}

// GetSubject : Gets the subject for this event
func (ev *Event) GetSubject() string {
	return ev.Subject
}

func (ev *Event) getRDSClient() *rds.RDS {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	return rds.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) setTags() error {
	svc := ev.getRDSClient()

	req := &rds.AddTagsToResourceInput{
		ResourceName: ev.ARN,
	}

	for key, val := range ev.Tags {
		req.Tags = append(req.Tags, &rds.Tag{
			Key:   &key,
			Value: &val,
		})
	}

	_, err := svc.AddTagsToResource(req)

	return err
}

func createSubnetGroup(ev *Event) (*string, error) {
	svc := ev.getRDSClient()

	if len(ev.NetworkAWSIDs) < 1 {
		return nil, nil
	}

	req := &rds.CreateDBSubnetGroupInput{
		DBSubnetGroupDescription: aws.String(*ev.Name + "-sg"),
		DBSubnetGroupName:        aws.String(*ev.Name + "-sg"),
		SubnetIds:                ev.NetworkAWSIDs,
	}

	_, err := svc.CreateDBSubnetGroup(req)

	return req.DBSubnetGroupName, err
}

func updateSubnetGroup(ev *Event) (*string, error) {
	svc := ev.getRDSClient()

	if len(ev.NetworkAWSIDs) < 1 {
		return nil, nil
	}

	req := &rds.ModifyDBSubnetGroupInput{
		DBSubnetGroupName:        aws.String(*ev.Name + "-sg"),
		DBSubnetGroupDescription: aws.String(*ev.Name + "-sg"),
		SubnetIds:                ev.NetworkAWSIDs,
	}

	_, err := svc.ModifyDBSubnetGroup(req)

	return req.DBSubnetGroupName, err
}

func deleteSubnetGroup(ev *Event) error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBSubnetGroupInput{
		DBSubnetGroupName: aws.String(*ev.Name + "-sg"),
	}

	_, err := svc.DeleteDBSubnetGroup(req)

	return err
}

func waitUntilClusterDeleted(ev *Event) {
	svc := ev.getRDSClient()

	req := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: ev.Name,
	}

	for {
		_, err := svc.DescribeDBClusters(req)
		if err != nil {
			return
		}

		time.Sleep(time.Second * 2)
	}
}
