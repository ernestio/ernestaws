/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package route53

import (
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
	uuid "github.com/satori/go.uuid"
)

var (
	// ErrDatacenterIDInvalid : error for invalid datacenter id
	ErrDatacenterIDInvalid = errors.New("Datacenter VPC ID invalid")
	// ErrDatacenterRegionInvalid : error for datacenter revgion invalid
	ErrDatacenterRegionInvalid = errors.New("Datacenter Region invalid")
	// ErrDatacenterCredentialsInvalid : error for datacenter credentials invalid
	ErrDatacenterCredentialsInvalid = errors.New("Datacenter credentials invalid")
	// ErrZoneNameInvalid : error for zone name invalid
	ErrZoneNameInvalid = errors.New("Route53 zone name invalid")
)

// Records stores a collection of records
type Records []Record

// HasRecord returns true if a matched entry is found
func (r Records) HasRecord(entry string) bool {
	// check with removed . character as well
	for _, record := range r {
		if entryName(*record.Entry) == entryName(entry) {
			return true
		}
	}
	return false
}

// Record stores the entries for a zone
type Record struct {
	Entry  *string   `json:"entry"`
	Type   *string   `json:"type"`
	Values []*string `json:"values"`
	TTL    *int64    `json:"ttl"`
}

// Event stores the template data
type Event struct {
	ProviderType     string            `json:"_provider"`
	ComponentType    string            `json:"_component"`
	ComponentID      string            `json:"_component_id"`
	State            string            `json:"_state"`
	Action           string            `json:"_action"`
	HostedZoneID     *string           `json:"hosted_zone_id"`
	Name             *string           `json:"name"`
	Private          *bool             `json:"private"`
	Records          Records           `json:"records"`
	VpcID            *string           `json:"vpc_id"`
	Tags             map[string]string `json:"tags"`
	DatacenterType   string            `json:"datacenter_type"`
	DatacenterName   string            `json:"datacenter_name"`
	DatacenterRegion string            `json:"datacenter_region"`
	AccessKeyID      string            `json:"aws_access_key_id"`
	SecretAccessKey  string            `json:"aws_secret_access_key"`
	Service          string            `json:"service"`
	ErrorMessage     string            `json:"error,omitempty"`
	Subject          string            `json:"-"`
	Body             []byte            `json:"-"`
	CryptoKey        string            `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	if strings.Split(subject, ".")[1] == "find" {
		return &Collection{Subject: subject, Body: body, CryptoKey: cryptoKey}
	}

	return &Event{Subject: subject, Body: body, CryptoKey: cryptoKey}
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

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AccessKeyID == "" || ev.SecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Name == nil {
		return ErrZoneNameInvalid
	}

	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a route53 object on aws
func (ev *Event) Create() error {
	svc := ev.getRoute53Client()

	req := &route53.CreateHostedZoneInput{
		CallerReference: aws.String(uuid.NewV4().String()),
		Name:            ev.Name,
	}

	if *ev.Private == true {
		req.HostedZoneConfig = &route53.HostedZoneConfig{
			PrivateZone: ev.Private,
		}
		req.VPC = &route53.VPC{
			VPCId:     ev.VpcID,
			VPCRegion: aws.String(ev.DatacenterRegion),
		}
	}

	resp, err := svc.CreateHostedZone(req)
	if err != nil {
		return err
	}

	ev.HostedZoneID = resp.HostedZone.Id

	return ev.Update()
}

// Update : Updates a route53 object on aws
func (ev *Event) Update() error {
	svc := ev.getRoute53Client()

	zr, err := ev.getZoneRecords()
	if err != nil {
		return err
	}

	req := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: ev.buildChanges(zr),
		},
		HostedZoneId: ev.HostedZoneID,
	}

	_, err = svc.ChangeResourceRecordSets(req)
	if err != nil {
		return err
	}

	return ev.setTags()
}

// Delete : Deletes a route53 object on aws
func (ev *Event) Delete() error {
	// clear ruleset before delete
	ev.Records = nil
	err := ev.Update()
	if err != nil {
		return err
	}

	svc := ev.getRoute53Client()

	req := &route53.DeleteHostedZoneInput{
		Id: ev.HostedZoneID,
	}

	_, err = svc.DeleteHostedZone(req)

	return err
}

// Get : Gets a route53 object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) getRoute53Client() *route53.Route53 {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	return route53.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) getZoneRecords() ([]*route53.ResourceRecordSet, error) {
	svc := ev.getRoute53Client()

	req := &route53.ListResourceRecordSetsInput{
		HostedZoneId: ev.HostedZoneID,
	}

	resp, err := svc.ListResourceRecordSets(req)
	if err != nil {
		return nil, err
	}

	return resp.ResourceRecordSets, nil
}

func (ev *Event) buildResourceRecords(values []*string) []*route53.ResourceRecord {
	var records []*route53.ResourceRecord

	for _, v := range values {
		records = append(records, &route53.ResourceRecord{
			Value: v,
		})
	}

	return records
}

func isDefaultRule(name string, record *route53.ResourceRecordSet) bool {
	return entryName(*record.Name) == entryName(name) && *record.Type == "SOA" ||
		entryName(*record.Name) == entryName(name) && *record.Type == "NS"
}

func (ev *Event) buildRecordsToRemove(existing []*route53.ResourceRecordSet) []*route53.Change {
	// Dont delete the default NS and SOA rules
	// May conflict with non-default rules, needs testing

	var missing []*route53.Change

	for _, recordSet := range existing {

		if ev.Records.HasRecord(*recordSet.Name) != true && isDefaultRule(*ev.Name, recordSet) != true {
			missing = append(missing, &route53.Change{
				Action:            aws.String("DELETE"),
				ResourceRecordSet: recordSet,
			})
		}
	}

	return missing
}

func (ev *Event) buildChanges(existing []*route53.ResourceRecordSet) []*route53.Change {
	var changes []*route53.Change

	for _, record := range ev.Records {
		changes = append(changes, &route53.Change{
			Action: aws.String("UPSERT"),
			ResourceRecordSet: &route53.ResourceRecordSet{
				Name:            record.Entry,
				Type:            record.Type,
				TTL:             record.TTL,
				ResourceRecords: ev.buildResourceRecords(record.Values),
			},
		})
	}

	changes = append(changes, ev.buildRecordsToRemove(existing)...)

	return changes
}

func entryName(entry string) string {
	if string(entry[len(entry)-1]) == "." {
		return entry[:len(entry)-1]
	}
	return entry
}

func (ev *Event) setTags() error {
	svc := ev.getRoute53Client()

	req := &route53.ChangeTagsForResourceInput{
		ResourceId:   ev.HostedZoneID,
		ResourceType: aws.String("hostedzone"),
	}

	for key, val := range ev.Tags {
		req.AddTags = append(req.AddTags, &route53.Tag{
			Key:   &key,
			Value: &val,
		})
	}

	_, err := svc.ChangeTagsForResource(req)

	return err
}
