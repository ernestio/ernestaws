/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package rdsinstance

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"sort"
	"strings"

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
	// ErrRDSInstanceNameInvalid ...
	ErrRDSInstanceNameInvalid = errors.New("RDS instance name invalid")
	// ErrRDSInstanceEngineTypeInvalid ...
	ErrRDSInstanceEngineTypeInvalid = errors.New("RDS instance engine invalid")
	// ErrRDSInstanceSizeInvalid ...
	ErrRDSInstanceSizeInvalid = errors.New("RDS instance size invalid")
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
	Size                *string           `json:"size"`
	Engine              *string           `json:"engine"`
	EngineVersion       *string           `json:"engine_version,omitempty"`
	Port                *int64            `json:"port,omitempty"`
	Cluster             *string           `json:"cluster,omitempty"`
	Public              *bool             `json:"public"`
	Endpoint            *string           `json:"endpoint,omitempty"`
	MultiAZ             *bool             `json:"multi_az"`
	PromotionTier       *int64            `json:"promotion_tier,omitempty"`
	StorageType         *string           `json:"storage_type,omitempty"`
	StorageSize         *int64            `json:"storage_size,omitempty"`
	StorageIops         *int64            `json:"storage_iops,omitempty"`
	AvailabilityZone    *string           `json:"availability_zone,omitempty"`
	SecurityGroups      []string          `json:"security_groups"`
	SecurityGroupAWSIDs []*string         `json:"security_group_aws_ids"`
	Networks            []string          `json:"networks"`
	NetworkAWSIDs       []*string         `json:"network_aws_ids"`
	DatabaseName        *string           `json:"database_name,omitempty"`
	DatabaseUsername    *string           `json:"database_username,omitempty"`
	DatabasePassword    *string           `json:"database_password,omitempty"`
	AutoUpgrade         *bool             `json:"auto_upgrade"`
	BackupRetention     *int64            `json:"backup_retention,omitempty"`
	BackupWindow        *string           `json:"backup_window,omitempty"`
	MaintenanceWindow   *string           `json:"maintenance_window,omitempty"`
	FinalSnapshot       *bool             `json:"final_snapshot"`
	ReplicationSource   *string           `json:"replication_source,omitempty"`
	License             *string           `json:"license,omitempty"`
	Timezone            *string           `json:"timezone,omitempty"`
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
		return ErrRDSInstanceNameInvalid
	}

	if ev.Engine == nil {
		return ErrRDSInstanceEngineTypeInvalid
	}

	if ev.Size == nil {
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
	ev.State = "errored"

	ev.Body, err = json.Marshal(ev)
}

// Complete : sets the state of the event to completed
func (ev *Event) Complete() {
	ev.State = "completed"
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

	if ev.ReplicationSource != nil {
		return ev.createReplicaDB(svc, subnetGroup)
	}

	return ev.createPrimaryDB(svc, subnetGroup)
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	svc := ev.getRDSClient()

	err := updateSubnetGroup(ev)
	if err != nil {
		return err
	}

	req := &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier:       ev.Name,
		DBInstanceClass:            ev.Size,
		EngineVersion:              ev.EngineVersion,
		DBPortNumber:               ev.Port,
		AllocatedStorage:           ev.StorageSize,
		StorageType:                ev.StorageType,
		Iops:                       ev.StorageIops,
		MultiAZ:                    ev.MultiAZ,
		PromotionTier:              ev.PromotionTier,
		AutoMinorVersionUpgrade:    ev.AutoUpgrade,
		BackupRetentionPeriod:      ev.BackupRetention,
		PreferredBackupWindow:      ev.BackupWindow,
		PreferredMaintenanceWindow: ev.MaintenanceWindow,
		VpcSecurityGroupIds:        ev.SecurityGroupAWSIDs,
		MasterUserPassword:         ev.DatabasePassword,
		LicenseModel:               ev.License,
		PubliclyAccessible:         ev.Public,
		ApplyImmediately:           aws.Bool(true),
	}

	_, err = svc.ModifyDBInstance(req)

	return err
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: ev.Name,
	}

	if *ev.FinalSnapshot {
		req.FinalDBSnapshotIdentifier = aws.String(*ev.Name + "-Final-Snapshot")
	} else {
		req.SkipFinalSnapshot = aws.Bool(true)
	}

	_, err := svc.DeleteDBInstance(req)
	if err != nil {
		return err
	}

	waitreq := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: ev.Name,
	}

	err = svc.WaitUntilDBInstanceDeleted(waitreq)
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
		DBInstanceIdentifier:       ev.Name,
		DBInstanceClass:            ev.Size,
		Engine:                     ev.Engine,
		EngineVersion:              ev.EngineVersion,
		Port:                       ev.Port,
		DBClusterIdentifier:        ev.Cluster,
		AllocatedStorage:           ev.StorageSize,
		StorageType:                ev.StorageType,
		Iops:                       ev.StorageIops,
		MultiAZ:                    ev.MultiAZ,
		PromotionTier:              ev.PromotionTier,
		AvailabilityZone:           ev.AvailabilityZone,
		AutoMinorVersionUpgrade:    ev.AutoUpgrade,
		BackupRetentionPeriod:      ev.BackupRetention,
		PreferredBackupWindow:      ev.BackupWindow,
		PreferredMaintenanceWindow: ev.MaintenanceWindow,
		VpcSecurityGroupIds:        ev.SecurityGroupAWSIDs,
		DBName:                     ev.DatabaseName,
		MasterUsername:             ev.DatabaseUsername,
		MasterUserPassword:         ev.DatabasePassword,
		DBSubnetGroupName:          subnetGroup,
		LicenseModel:               ev.License,
		PubliclyAccessible:         ev.Public,
		Timezone:                   ev.Timezone,
	}

	_, err := svc.CreateDBInstance(req)
	if err != nil {
		return err
	}

	waitreq := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: ev.Name,
	}

	err = svc.WaitUntilDBInstanceAvailable(waitreq)
	if err != nil {
		return err
	}

	resp, err := svc.DescribeDBInstances(waitreq)
	if err != nil {
		return err
	}

	ev.ARN = resp.DBInstances[0].DBInstanceArn

	if resp.DBInstances[0].Endpoint != nil {
		if resp.DBInstances[0].Endpoint.Address != nil {
			ev.Endpoint = resp.DBInstances[0].Endpoint.Address
		}
	}

	return ev.setTags()
}

func (ev *Event) createReplicaDB(svc *rds.RDS, subnetGroup *string) error {
	req := &rds.CreateDBInstanceReadReplicaInput{
		AutoMinorVersionUpgrade:    ev.AutoUpgrade,
		AvailabilityZone:           ev.AvailabilityZone,
		DBInstanceIdentifier:       ev.Name,
		DBInstanceClass:            ev.Size,
		DBSubnetGroupName:          subnetGroup,
		StorageType:                ev.StorageType,
		Iops:                       ev.StorageIops,
		Port:                       ev.Port,
		PubliclyAccessible:         ev.Public,
		SourceDBInstanceIdentifier: ev.ReplicationSource,
	}

	_, err := svc.CreateDBInstanceReadReplica(req)
	if err != nil {
		return err
	}

	waitreq := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: ev.Name,
	}

	err = svc.WaitUntilDBInstanceAvailable(waitreq)
	if err != nil {
		return err
	}

	resp, err := svc.DescribeDBInstances(waitreq)
	if err != nil {
		return err
	}

	ev.ARN = resp.DBInstances[0].DBInstanceArn

	if resp.DBInstances[0].Endpoint != nil {
		ev.Endpoint = resp.DBInstances[0].Endpoint.Address
	}

	return ev.setTags()
}

func (ev *Event) getRDSClient() *rds.RDS {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
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
		DBSubnetGroupDescription: aws.String(*ev.Name + "-sg"),
		DBSubnetGroupName:        aws.String(*ev.Name + "-sg"),
		SubnetIds:                ev.NetworkAWSIDs,
	}

	_, err := svc.CreateDBSubnetGroup(req)

	return req.DBSubnetGroupName, err
}

func updateSubnetGroup(ev *Event) error {
	svc := ev.getRDSClient()

	if len(ev.NetworkAWSIDs) < 1 {
		return nil
	}

	sg, err := getSubnetGroup(ev)
	if err != nil {
		return err
	}

	if subnetsHaveChanged(ev.NetworkAWSIDs, sg.Subnets) != true {
		return nil
	}

	req := &rds.ModifyDBSubnetGroupInput{
		DBSubnetGroupName:        aws.String(*ev.Name + "-sg"),
		DBSubnetGroupDescription: aws.String(*ev.Name + "-sg"),
		SubnetIds:                ev.NetworkAWSIDs,
	}

	_, err = svc.ModifyDBSubnetGroup(req)

	return err
}

func subnetsHaveChanged(ids []*string, subnets []*rds.Subnet) bool {
	var sids []*string
	for _, s := range subnets {
		sids = append(sids, s.SubnetIdentifier)
	}

	if len(ids) != len(sids) {
		return true
	}

	idsx := ptrSliceToStrSlice(ids)
	sidsx := ptrSliceToStrSlice(sids)
	idsx.Sort()
	sidsx.Sort()

	return !reflect.DeepEqual(idsx, sidsx)
}

func getSubnetGroup(ev *Event) (*rds.DBSubnetGroup, error) {
	svc := ev.getRDSClient()

	req := &rds.DescribeDBSubnetGroupsInput{
		DBSubnetGroupName: aws.String(*ev.Name + "-sg"),
	}

	resp, err := svc.DescribeDBSubnetGroups(req)
	if err != nil {
		return nil, err
	}

	if len(resp.DBSubnetGroups) < 1 {
		return nil, err
	}

	return resp.DBSubnetGroups[0], nil
}

func deleteSubnetGroup(ev *Event) error {
	svc := ev.getRDSClient()

	req := &rds.DeleteDBSubnetGroupInput{
		DBSubnetGroupName: aws.String(*ev.Name + "-sg"),
	}

	_, err := svc.DeleteDBSubnetGroup(req)

	return err
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

func ptrSliceToStrSlice(s []*string) sort.StringSlice {
	var ds sort.StringSlice
	for _, str := range s {
		ds = append(ds, *str)
	}
	return ds
}
