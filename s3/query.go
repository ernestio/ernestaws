/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package s3

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

func getS3Client(q *ernestaws.Query) *s3.S3 {
	creds, _ := credentials.NewStaticCredentials(q.AWSAccessKeyID, q.AWSSecretAccessKey, q.CryptoKey)
	return s3.New(session.New(), &aws.Config{
		Region:      aws.String(q.DatacenterRegion),
		Credentials: creds,
	})
}

// FindS3Buckets : Find s3 buckets on aws
func FindS3Buckets(q *ernestaws.Query) error {
	svc := getS3Client(q)

	resp, err := svc.ListBuckets(nil)
	if err != nil {
		return err
	}

	for _, b := range resp.Buckets {
		tags, err := getBucketTagDescriptions(svc, b.Name)
		if err != nil {
			return err
		}

		grants, err := getBucketPermissions(svc, b.Name)
		if err != nil {
			return err
		}

		location, err := getBucketLocation(svc, b.Name)
		if err != nil {
			return err
		}

		event := ToEvent(b, grants, location, tags)

		if tagsMatch(q.Tags, event.Tags) {
			q.Results = append(q.Results, event)
		}
	}

	return nil
}

func mapS3Tags(input []*s3.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func tagsMatch(qt, rt map[string]string) bool {
	for k, v := range qt {
		if rt[k] != v {
			return false
		}
	}

	return true
}

func getBucketTagDescriptions(svc *s3.S3, name *string) ([]*s3.Tag, error) {
	treq := &s3.GetBucketTaggingInput{
		Bucket: name,
	}

	resp, err := svc.GetBucketTagging(treq)

	return resp.TagSet, err
}

func getBucketPermissions(svc *s3.S3, name *string) ([]*s3.Grant, error) {
	req := &s3.GetBucketAclInput{
		Bucket: name,
	}

	resp, err := svc.GetBucketAcl(req)

	return resp.Grants, err
}

func getBucketLocation(svc *s3.S3, name *string) (*string, error) {
	req := &s3.GetBucketLocationInput{
		Bucket: name,
	}

	resp, err := svc.GetBucketLocation(req)

	return resp.LocationConstraint, err
}

func mapS3Grantees(grantees []*s3.Grant) []Grantee {
	var gs []Grantee

	for _, g := range grantees {
		grantee := Grantee{
			ID:          *g.Grantee.ID,
			Type:        *g.Grantee.Type,
			Permissions: *g.Permission,
		}

		switch *g.Grantee.Type {
		case "id":
			grantee.ID = *g.Grantee.ID
		case "emailaddress":
			grantee.ID = *g.Grantee.EmailAddress
		case "uri":
			grantee.ID = *g.Grantee.URI
		}

		gs = append(gs, grantee)
	}

	return gs
}

// ToEvent converts an s3 bucket object to an ernest event
func ToEvent(b *s3.Bucket, grants []*s3.Grant, location *string, tags []*s3.Tag) *Event {
	e := &Event{
		Name:           *b.Name,
		Grantees:       mapS3Grantees(grants),
		BucketLocation: *location,
		// ACL ???
		// BucketURI
		Tags: mapS3Tags(tags),
	}
	return e
}
