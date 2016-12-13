/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdscluster

import (
	"encoding/json"
	"errors"
	"log"
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
	UUID                  string    `json:"_uuid"`
	BatchID               string    `json:"_batch_id"`
	ProviderType          string    `json:"_type"`
	DatacenterRegion      string    `json:"datacenter_region"`
	DatacenterAccessKey   string    `json:"datacenter_secret"`
	DatacenterAccessToken string    `json:"datacenter_token"`
	VPCID                 string    `json:"vpc_id"`
	Name                  string    `json:"name"`
	Engine                string    `json:"engine"`
	EngineVersion         *string   `json:"engine_version"`
	Port                  *int64    `json:"port"`
	Endpoint              string    `json:"endpoint"`
	AvailabilityZones     []*string `json:"availability_zones"`
	SecurityGroups        []string  `json:"security_groups"`
	SecurityGroupAWSIDs   []*string `json:"security_group_aws_ids"`
	Networks              []string  `json:"networks"`
	NetworkAWSIDs         []*string `json:"network_aws_ids"`
	DatabaseName          *string   `json:"database_name"`
	DatabaseUsername      *string   `json:"database_username"`
	DatabasePassword      *string   `json:"database_password"`
	BackupRetention       *int64    `json:"backup_retention"`
	BackupWindow          *string   `json:"backup_window"`
	MaintenanceWindow     *string   `json:"maintenance_window"`
	ReplicationSource     *string   `json:"replication_source"`
	FinalSnapshot         bool      `json:"final_snapshot"`
	ErrorMessage          string    `json:"error,omitempty"`
	Subject               string    `json:"-"`
	Body                  []byte    `json:"-"`
	CryptoKey             string    `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	return &Event{Subject: subject, Body: body, CryptoKey: cryptoKey}
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.VPCID == "" {
		return ErrDatacenterIDInvalid
	}

	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.DatacenterAccessKey == "" || ev.DatacenterAccessToken == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Name == "" {
		return ErrRDSClusterNameInvalid
	}

	if ev.Engine == "" {
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

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	svc := ev.getRDSClient()

	subnetGroup, err := createSubnetGroup(ev)
	if err != nil {
		return err
	}

	req := &rds.CreateDBClusterInput{
		DBClusterIdentifier:         aws.String(ev.Name),
		Engine:                      aws.String(ev.Engine),
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

	ev.Endpoint = *resp.DBCluster.Endpoint

	return nil
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	svc := ev.getRDSClient()

	_, err := updateSubnetGroup(ev)
	if err != nil {
		return err
	}

	req := &rds.ModifyDBClusterInput{
		DBClusterIdentifier:        aws.String(ev.Name),
		Port:                       ev.Port,
		MasterUserPassword:         ev.DatabasePassword,
		BackupRetentionPeriod:      ev.BackupRetention,
		PreferredBackupWindow:      ev.BackupWindow,
		PreferredMaintenanceWindow: ev.MaintenanceWindow,
		VpcSecurityGroupIds:        ev.SecurityGroupAWSIDs,
		ApplyImmediately:           aws.Bool(true),
	}

	_, err = svc.ModifyDBCluster(req)

	return err
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBClusterInput{
		DBClusterIdentifier: aws.String(ev.Name),
	}

	if ev.FinalSnapshot {
		req.FinalDBSnapshotIdentifier = aws.String(ev.Name + "-Final-Snapshot")
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
	creds, _ := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, ev.CryptoKey)
	return rds.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func createSubnetGroup(ev *Event) (*string, error) {
	svc := ev.getRDSClient()

	if len(ev.NetworkAWSIDs) < 1 {
		return nil, nil
	}

	req := &rds.CreateDBSubnetGroupInput{
		DBSubnetGroupDescription: aws.String(ev.Name + "-sg"),
		DBSubnetGroupName:        aws.String(ev.Name + "-sg"),
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
		DBSubnetGroupName:        aws.String(ev.Name + "-sg"),
		DBSubnetGroupDescription: aws.String(ev.Name + "-sg"),
		SubnetIds:                ev.NetworkAWSIDs,
	}

	_, err := svc.ModifyDBSubnetGroup(req)

	return req.DBSubnetGroupName, err
}

func deleteSubnetGroup(ev *Event) error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBSubnetGroupInput{
		DBSubnetGroupName: aws.String(ev.Name + "-sg"),
	}

	_, err := svc.DeleteDBSubnetGroup(req)

	return err
}

func waitUntilClusterDeleted(ev *Event) {
	svc := ev.getRDSClient()

	req := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(ev.Name),
	}

	for {
		_, err := svc.DescribeDBClusters(req)
		if err != nil {
			return
		}

		time.Sleep(time.Second * 2)
	}
}
