/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package route53

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/ernestio/ernestaws"
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
		if entryName(record.Entry) == entryName(entry) {
			return true
		}
	}
	return false
}

// Record stores the entries for a zone
type Record struct {
	Entry  string   `json:"entry"`
	Type   string   `json:"type"`
	Values []string `json:"values"`
	TTL    int64    `json:"ttl"`
}

// Event stores the template data
type Event struct {
	UUID             string  `json:"_uuid"`
	BatchID          string  `json:"_batch_id"`
	ProviderType     string  `json:"_type"`
	HostedZoneID     string  `json:"hosted_zone_id"`
	Name             string  `json:"name"`
	Private          bool    `json:"private"`
	Records          Records `json:"records"`
	VPCID            string  `json:"vpc_id"`
	DatacenterName   string  `json:"datacenter_name,omitempty"`
	DatacenterRegion string  `json:"datacenter_region"`
	DatacenterToken  string  `json:"datacenter_token"`
	DatacenterSecret string  `json:"datacenter_secret"`
	ErrorMessage     string  `json:"error_message,omitempty"`
	Subject          string  `json:"-"`
	Body             []byte  `json:"-"`
}

// New : Constructor
func New(subject string, body []byte) ernestaws.Event {
	n := Event{Subject: subject, Body: body}

	return &n
}

// GetBody : Gets the body for this event
func (ev *Event) GetBody() []byte {
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

	ev.Body, err = json.Marshal(ev)
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.VPCID == "" {
		return ErrDatacenterIDInvalid
	}

	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.DatacenterSecret == "" || ev.DatacenterToken == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Name == "" {
		return ErrZoneNameInvalid
	}

	return nil
}

// Create : Creates a route53 object on aws
func (ev *Event) Create() error {
	svc := ev.getRoute53Client()

	req := &route53.CreateHostedZoneInput{
		CallerReference: aws.String(uuid.NewV4().String()),
		Name:            aws.String(ev.Name),
	}

	if ev.Private == true {
		req.HostedZoneConfig = &route53.HostedZoneConfig{
			PrivateZone: aws.Bool(ev.Private),
		}
		req.VPC = &route53.VPC{
			VPCId:     aws.String(ev.VPCID),
			VPCRegion: aws.String(ev.DatacenterRegion),
		}
	}

	resp, err := svc.CreateHostedZone(req)
	if err != nil {
		return err
	}

	ev.HostedZoneID = *resp.HostedZone.Id

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
		HostedZoneId: aws.String(ev.HostedZoneID),
	}

	_, err = svc.ChangeResourceRecordSets(req)
	if err != nil {
		return err
	}

	return err
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
		Id: aws.String(ev.HostedZoneID),
	}

	_, err = svc.DeleteHostedZone(req)

	return err
}

// Get : Gets a route53 object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) getRoute53Client() *route53.Route53 {
	creds := credentials.NewStaticCredentials(ev.DatacenterSecret, ev.DatacenterToken, "")
	return route53.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) getZoneRecords() ([]*route53.ResourceRecordSet, error) {
	svc := ev.getRoute53Client()

	req := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(ev.HostedZoneID),
	}

	resp, err := svc.ListResourceRecordSets(req)
	if err != nil {
		return nil, err
	}

	return resp.ResourceRecordSets, nil
}

func (ev *Event) buildResourceRecords(values []string) []*route53.ResourceRecord {
	var records []*route53.ResourceRecord

	for _, v := range values {
		records = append(records, &route53.ResourceRecord{
			Value: aws.String(v),
		})
	}

	return records
}

func (ev *Event) isDefaultRule(name string, record *route53.ResourceRecordSet) bool {
	return entryName(*record.Name) == entryName(name) && *record.Type == "SOA" ||
		entryName(*record.Name) == entryName(name) && *record.Type == "NS"
}

func (ev *Event) buildRecordsToRemove(existing []*route53.ResourceRecordSet) []*route53.Change {
	// Dont delete the default NS and SOA rules
	// May conflict with non-default rules, needs testing

	var missing []*route53.Change

	for _, recordSet := range existing {

		if ev.Records.HasRecord(*recordSet.Name) != true && ev.isDefaultRule(ev.Name, recordSet) != true {
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
				Name:            aws.String(record.Entry),
				Type:            aws.String(record.Type),
				TTL:             aws.Int64(record.TTL),
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
