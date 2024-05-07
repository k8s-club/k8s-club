package client

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestSecretData(t *testing.T) {
	secret, err := ClientSet.Client.CoreV1().Secrets("kube-system").Get(context.TODO(), "xxx", metav1.GetOptions{})
	fmt.Println(secret.Data, err)

	if len(secret.Data) > 0 {
		for k, v := range secret.Data {
			fmt.Println("=======", k, string(v)) // base64 decoded value
		}
	}
}
