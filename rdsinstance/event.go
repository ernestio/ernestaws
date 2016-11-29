/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdsinstance

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/ernestio/ernestaws"
)

var (
	// ErrDatacenterIDInvalid ...
	ErrDatacenterIDInvalid = errors.New("Datacenter VPC ID invalid")
	// ErrDatacenterRegionInvalid ...
	ErrDatacenterRegionInvalid = errors.New("Datacenter Region invalid")
	// ErrDatacenterCredentialsInvalid ...
	ErrDatacenterCredentialsInvalid = errors.New("Datacenter credentials invalid")
	// ErrRDSInstanceNameInvalid ...
	ErrRDSInstanceNameInvalid = errors.New("RDS instance name invalid")
	// ErrRDSInstanceEngineTypeInvalid ...
	ErrRDSInstanceEngineTypeInvalid = errors.New("RDS instance engine invalid")
	// ErrRDSInstanceSizeInvalid ...
	ErrRDSInstanceSizeInvalid = errors.New("RDS instance size invalid")
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
	Size                  string    `json:"size"`
	Engine                string    `json:"engine"`
	EngineVersion         string    `json:"engine_version"`
	Port                  *int64    `json:"port"`
	Cluster               string    `json:"cluster"`
	Public                bool      `json:"public"`
	Endpoint              string    `json:"endpoint"`
	MultiAZ               bool      `json:"multi_az"`
	PromotionTier         *int64    `json:"promotion_tier"`
	StorageType           string    `json:"storage_type"`
	StorageSize           *int64    `json:"storage_size"`
	StorageIops           *int64    `json:"storage_iops"`
	AvailabilityZone      string    `json:"availability_zone"`
	SecurityGroups        []string  `json:"security_groups"`
	SecurityGroupAWSIDs   []*string `json:"security_group_aws_ids"`
	Networks              []string  `json:"networks"`
	NetworkAWSIDs         []*string `json:"network_aws_ids"`
	DatabaseName          string    `json:"database_name"`
	DatabaseUsername      string    `json:"database_username"`
	DatabasePassword      string    `json:"database_password"`
	AutoUpgrade           bool      `json:"auto_upgrade"`
	BackupRetention       int64     `json:"backup_retention"`
	BackupWindow          string    `json:"backup_window"`
	MaintenanceWindow     string    `json:"maintenance_window"`
	FinalSnapshot         bool      `json:"final_snapshot"`
	ReplicationSource     string    `json:"replication_source"`
	License               string    `json:"license"`
	Timezone              string    `json:"timezone"`
	ErrorMessage          string    `json:"error,omitempty"`
	Subject               string    `json:"-"`
	Body                  []byte    `json:"-"`
}

// New : Constructor
func New(subject string, body []byte) ernestaws.Event {
	return &Event{Subject: subject, Body: body}
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
		return ErrRDSInstanceNameInvalid
	}

	if ev.Engine == "" {
		return ErrRDSInstanceEngineTypeInvalid
	}

	if ev.Size == "" {
		return ErrRDSInstanceSizeInvalid
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

	if ev.ReplicationSource != "" {
		return ev.createReplicaDB(svc, subnetGroup)
	}

	return ev.createPrimaryDB(svc, subnetGroup)
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	svc := ev.getRDSClient()

	subnetGroup, err := updateSubnetGroup(ev)
	if err != nil {
		return err
	}

	req := &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier:       aws.String(ev.Name),
		DBInstanceClass:            aws.String(ev.Size),
		EngineVersion:              aws.String(ev.EngineVersion),
		DBPortNumber:               ev.Port,
		AllocatedStorage:           ev.StorageSize,
		StorageType:                aws.String(ev.StorageType),
		Iops:                       ev.StorageIops,
		MultiAZ:                    aws.Bool(ev.MultiAZ),
		PromotionTier:              ev.PromotionTier,
		AutoMinorVersionUpgrade:    aws.Bool(ev.AutoUpgrade),
		BackupRetentionPeriod:      aws.Int64(ev.BackupRetention),
		PreferredBackupWindow:      aws.String(ev.BackupWindow),
		PreferredMaintenanceWindow: aws.String(ev.MaintenanceWindow),
		VpcSecurityGroupIds:        ev.SecurityGroupAWSIDs,
		MasterUserPassword:         aws.String(ev.DatabasePassword),
		DBSubnetGroupName:          subnetGroup,
		LicenseModel:               aws.String(ev.License),
		PubliclyAccessible:         aws.Bool(ev.Public),
		ApplyImmediately:           aws.Bool(true),
	}

	_, err = svc.ModifyDBInstance(req)

	return err
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(ev.Name),
	}

	if ev.FinalSnapshot {
		req.FinalDBSnapshotIdentifier = aws.String(ev.Name + "-Final-Snapshot")
	} else {
		req.SkipFinalSnapshot = aws.Bool(true)
	}

	_, err := svc.DeleteDBInstance(req)
	if err != nil {
		return err
	}

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

func (ev *Event) createPrimaryDB(svc *rds.RDS, subnetGroup *string) error {
	req := &rds.CreateDBInstanceInput{
		DBInstanceIdentifier:       aws.String(ev.Name),
		DBInstanceClass:            aws.String(ev.Size),
		Engine:                     aws.String(ev.Engine),
		EngineVersion:              aws.String(ev.EngineVersion),
		Port:                       ev.Port,
		DBClusterIdentifier:        aws.String(ev.Cluster),
		AllocatedStorage:           ev.StorageSize,
		StorageType:                aws.String(ev.StorageType),
		Iops:                       ev.StorageIops,
		MultiAZ:                    aws.Bool(ev.MultiAZ),
		PromotionTier:              ev.PromotionTier,
		AvailabilityZone:           aws.String(ev.AvailabilityZone),
		AutoMinorVersionUpgrade:    aws.Bool(ev.AutoUpgrade),
		BackupRetentionPeriod:      aws.Int64(ev.BackupRetention),
		PreferredBackupWindow:      aws.String(ev.BackupWindow),
		PreferredMaintenanceWindow: aws.String(ev.MaintenanceWindow),
		VpcSecurityGroupIds:        ev.SecurityGroupAWSIDs,
		DBName:                     aws.String(ev.DatabaseName),
		MasterUsername:             aws.String(ev.DatabaseUsername),
		MasterUserPassword:         aws.String(ev.DatabasePassword),
		DBSubnetGroupName:          subnetGroup,
		LicenseModel:               aws.String(ev.License),
		PubliclyAccessible:         aws.Bool(ev.Public),
		Timezone:                   aws.String(ev.Timezone),
	}

	resp, err := svc.CreateDBInstance(req)
	if err != nil {
		return err
	}

	ev.Endpoint = *resp.DBInstance.Endpoint.Address

	return nil
}

func (ev *Event) createReplicaDB(svc *rds.RDS, subnetGroup *string) error {
	req := &rds.CreateDBInstanceReadReplicaInput{
		AutoMinorVersionUpgrade:    aws.Bool(ev.AutoUpgrade),
		AvailabilityZone:           aws.String(ev.AvailabilityZone),
		DBInstanceIdentifier:       aws.String(ev.Name),
		DBInstanceClass:            aws.String(ev.Size),
		DBSubnetGroupName:          subnetGroup,
		StorageType:                aws.String(ev.StorageType),
		Iops:                       ev.StorageIops,
		Port:                       ev.Port,
		PubliclyAccessible:         aws.Bool(ev.Public),
		SourceDBInstanceIdentifier: aws.String(ev.ReplicationSource),
	}

	resp, err := svc.CreateDBInstanceReadReplica(req)
	if err != nil {
		return err
	}

	ev.Endpoint = *resp.DBInstance.Endpoint.Address

	return nil
}

func (ev *Event) getRDSClient() *rds.RDS {
	creds := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, "")
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
