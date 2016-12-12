package credentials

import (
	"log"

	"github.com/aws/aws-sdk-go/aws/credentials"
	aes "github.com/ernestio/crypto/aes"
)

// NewStaticCredentials : Get the aws credentials object based on a
// encrypted token and secret pair
func NewStaticCredentials(encryptedToken, encryptedSecret string, cryptoKey []byte) (*credentials.Credentials, error) {
	var err error
	var token, secret []byte

	if string(cryptoKey) == "" {
		token = []byte(encryptedToken)
		secret = []byte(encryptedSecret)
	} else {
		crypto := aes.New()
		if token, err = crypto.Decrypt([]byte(encryptedToken), cryptoKey); err != nil {
			log.Println(err.Error())
			return nil, err
		}
		if secret, err = crypto.Decrypt([]byte(encryptedSecret), cryptoKey); err != nil {
			log.Println(err.Error())
			return nil, err
		}
	}

	return credentials.NewStaticCredentials(string(secret), string(token), ""), nil
}
