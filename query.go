/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ernestaws

// Query ....
type Query struct {
	UUID               string            `json:"_uuid"`
	BatchID            string            `json:"_batch_id"`
	ProviderType       string            `json:"_type"`
	AWSAccessKeyID     string            `json:"aws_access_key_id"`
	AWSSecretAccessKey string            `json:"aws_secret_access_key"`
	DatacenterRegion   string            `json:"datacenter_region"`
	Tags               map[string]string `json:"tags"`
	Results            []interface{}     `json:"results"`
	CryptoKey          string            `json:"-"`
}
