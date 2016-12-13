package credentials

import (
	"log"

	"github.com/aws/aws-sdk-go/aws/credentials"
	aes "github.com/ernestio/crypto/aes"
)

// NewStaticCredentials : Get the aws credentials object based on a
// encrypted token and secret pair
func NewStaticCredentials(token, secret, cryptoKey string) (*credentials.Credentials, error) {
	var err error

	if cryptoKey != "" {
		crypto := aes.New()
		if token, err = crypto.Decrypt(token, cryptoKey); err != nil {
			log.Println(err.Error())
			return nil, err
		}
		if secret, err = crypto.Decrypt(secret, cryptoKey); err != nil {
			log.Println(err.Error())
			return nil, err
		}
	}

	return credentials.NewStaticCredentials(secret, token, ""), nil
}
