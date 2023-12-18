// This file has been modified to enable service account token generation
// using the RBAC v1 API. Credit to the original authors at Rancher.
// https://github.com/rancher/kontainer-engine/blob/release/v2.4/drivers/util/utils.go

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	cattleNamespace           = "cattle-system"
	clusterAdmin              = "cluster-admin"
	kontainerEngine           = "kontainer-engine"
	newClusterRoleBindingName = "system-netes-default-clusterRoleBinding"
	serviceAccountSecretName  = "kontainer-engine-secret"
)

func generateServiceAccountToken(clientset kubernetes.Interface) (string, error) {
	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cattleNamespace,
		},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", err
	}

	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: kontainerEngine,
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(cattleNamespace).Create(context.TODO(), serviceAccount, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("error creating service account: %v", err)
	}

	adminRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterAdmin,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
			{
				NonResourceURLs: []string{"*"},
				Verbs:           []string{"*"},
			},
		},
	}
	clusterAdminRole, err := clientset.RbacV1().ClusterRoles().Get(context.TODO(), clusterAdmin, metav1.GetOptions{})
	if err != nil {
		clusterAdminRole, err = clientset.RbacV1().ClusterRoles().Create(context.TODO(), adminRole, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("error creating admin role: %v", err)
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: newClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount.Name,
				Namespace: cattleNamespace,
				APIGroup:  v1.GroupName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterAdminRole.Name,
			APIGroup: rbacv1.GroupName,
		},
	}
	if _, err = clientset.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("error creating role bindings: %v", err)
	}

	// Create a service account token secret
	secret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccountSecretName,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": kontainerEngine,
			},
		},
		Type: v1.SecretTypeServiceAccountToken,
	}

	_, err = clientset.CoreV1().Secrets(cattleNamespace).Create(
		context.TODO(),
		&secret,
		metav1.CreateOptions{},
	)
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf(
			"failed to create secret for service account %s: %w",
			serviceAccount.Name,
			err,
		)
	}

	return waitForServiceAccountSecretPopulated(clientset)
}

// waitForServiceAccountSecretPopulated waits for the cattle service account
// token to be populated.
func waitForServiceAccountSecretPopulated(clientset kubernetes.Interface) (string, error) {
	var result string

	err := wait.PollImmediate(time.Millisecond*500, time.Second*15, func() (done bool, err error) {
		refreshedSecret, err := clientset.CoreV1().Secrets(cattleNamespace).Get(
			context.TODO(),
			serviceAccountSecretName,
			metav1.GetOptions{},
		)
		if err != nil {
			return false, fmt.Errorf("failed to refresh secret: %w", err)
		}

		token, ok := refreshedSecret.Data["token"]
		if !ok {
			logrus.Debugf("token is not yet available, retrying")
			return false, nil
		}

		result = string(token)
		return true, nil
	})

	return result, err
}
