/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package s3

import (
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
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
	// ErrS3NameInvalid ...
	ErrS3NameInvalid = errors.New("S3 bucket name is invalid")
)

// Grantee ...
type Grantee struct {
	ID          *string `json:"id"`
	Type        *string `json:"type"`
	Permissions *string `json:"permissions"`
}

// Event stores the template data
type Event struct {
	ProviderType     string            `json:"_provider"`
	ComponentType    string            `json:"_component"`
	ComponentID      string            `json:"_component_id"`
	State            string            `json:"_state"`
	Action           string            `json:"_action"`
	Name             *string           `json:"name"`
	ACL              *string           `json:"acl"`
	BucketLocation   *string           `json:"bucket_location"`
	BucketURI        *string           `json:"bucket_uri"`
	Grantees         []Grantee         `json:"grantees,omitempty"`
	Tags             map[string]string `json:"tags"`
	DatacenterType   string            `json:"datacenter_type,omitempty"`
	DatacenterName   string            `json:"datacenter_name,omitempty"`
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
		return ErrS3NameInvalid
	}

	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	s3client := ev.getS3Client()

	params := &s3.CreateBucketInput{
		Bucket: ev.Name,
		ACL:    ev.ACL,
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: ev.BucketLocation,
		},
	}

	resp, err := s3client.CreateBucket(params)
	if err != nil {
		return err
	}

	ev.BucketURI = resp.Location

	if len(ev.Grantees) < 1 {
		return ev.setTags()
	}

	return ev.Update()
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	s3client := ev.getS3Client()
	params := &s3.PutBucketAclInput{
		Bucket: ev.Name,
	}

	if stringEmpty(ev.ACL) != true {
		params.ACL = ev.ACL
	}

	var grants []*s3.Grant

	for _, g := range ev.Grantees {
		var grantee s3.Grantee

		switch *g.Type {
		case "id", "CanonicalUser":
			grantee.Type = aws.String(s3.TypeCanonicalUser)
			grantee.ID = g.ID
		case "emailaddress", "AmazonCustomerByEmail":
			grantee.Type = aws.String(s3.TypeAmazonCustomerByEmail)
			grantee.EmailAddress = g.ID
		case "uri", "Group":
			grantee.Type = aws.String(s3.TypeGroup)
			grantee.URI = g.ID
		}

		grants = append(grants, &s3.Grant{
			Grantee:    &grantee,
			Permission: g.Permissions,
		})
	}

	if stringEmpty(ev.ACL) {
		grt, err := ev.getACL()
		if err != nil {
			return err
		}

		params.AccessControlPolicy = &s3.AccessControlPolicy{
			Grants: grants,
			Owner:  grt.Owner,
		}
	}

	_, err := s3client.PutBucketAcl(params)
	if err != nil {
		return err
	}

	return ev.setTags()
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	s3client := ev.getS3Client()
	params := &s3.DeleteBucketInput{
		Bucket: ev.Name,
	}
	_, err := s3client.DeleteBucket(params)

	return err
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) getS3Client() *s3.S3 {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	s3client := s3.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
	return s3client
}

func (ev *Event) getACL() (*s3.GetBucketAclOutput, error) {
	s3client := ev.getS3Client()

	params := &s3.GetBucketAclInput{
		Bucket: ev.Name,
	}

	resp, err := s3client.GetBucketAcl(params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (ev *Event) setTags() error {
	svc := ev.getS3Client()

	if len(ev.Tags) < 1 {
		return nil
	}

	req := &s3.PutBucketTaggingInput{
		Bucket: ev.Name,
	}

	tags := s3.Tagging{}

	for key, val := range ev.Tags {
		tags.TagSet = append(tags.TagSet, &s3.Tag{
			Key:   &key,
			Value: &val,
		})
	}

	req.Tagging = &tags

	_, err := svc.PutBucketTagging(req)

	return err
}

func stringEmpty(s *string) bool {
	if s == nil {
		return true
	}

	if *s == "" {
		return true
	}

	return false
}
