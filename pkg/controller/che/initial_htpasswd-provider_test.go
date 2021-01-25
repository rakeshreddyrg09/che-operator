//
// Copyright (c) 2012-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package che

import (
	"context"
	"os"
	"testing"
	mocks "github.com/eclipse/che-operator/mocks"
	orgv1 "github.com/eclipse/che-operator/pkg/apis/org/v1"
	"github.com/golang/mock/gomock"
	oauth_config "github.com/openshift/api/config/v1"
	userv1 "github.com/openshift/api/user/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	testNamespace = "test-namespace"
)

func TestCreateInitialUser(t *testing.T) {
	type testCase struct {
		name                  string
		oAuth *oauth_config.OAuth
		initObjects           []runtime.Object
	}

	oAuth := &oauth_config.OAuth{
		ObjectMeta: v1.ObjectMeta{
			Name: "cluster",
		},
		Spec: oauth_config.OAuthSpec{IdentityProviders: []oauth_config.IdentityProvider{}},
	}

	logf.SetLogger(zap.LoggerTo(os.Stdout, true))

	scheme := scheme.Scheme
	orgv1.SchemeBuilder.AddToScheme(scheme)
	scheme.AddKnownTypes(userv1.SchemeGroupVersion, &userv1.UserList{}, &userv1.User{})
	scheme.AddKnownTypes(oauth_config.SchemeGroupVersion, &oauth_config.OAuth{})

	runtimeClient := fake.NewFakeClientWithScheme(scheme, oAuth)

	ctrl := gomock.NewController(t)
	m := mocks.NewMockRunnable(ctrl)
	m.EXPECT().Run("htpasswd", "-nbB", gomock.Any(), gomock.Any()).Return(nil)
	m.EXPECT().GetStdOut().Return("test-string") 
	m.EXPECT().GetStdErr().Return("")
	defer ctrl.Finish()

	initialUserHandler := &InitialUserOperatorHandler{
		runtimeClient: runtimeClient,
		runnable: m,
	}
	err := initialUserHandler.CreateOauthInitialUser(testNamespace, oAuth); if err != nil {
		t.Errorf("Failed to create user: %s", err.Error())
	}

	// Check created objects
	expectedCheSecret := &corev1.Secret{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: initialUserSecret, Namespace: testNamespace}, expectedCheSecret); err != nil {
		t.Errorf("Initial user secret should exists")
	}

	expectedHtpasswsSecret := &corev1.Secret{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: htpasswdSecretName, Namespace: ocConfigNamespace}, expectedHtpasswsSecret); err != nil {
		t.Errorf("Initial user secret should exists")
	}

	expectedOAuth := &oauth_config.OAuth{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, expectedOAuth); err != nil {
		t.Errorf("Initial oAuth should exists")
	}

	if len(expectedOAuth.Spec.IdentityProviders) < 0 {
		t.Error("List identity providers should not be an empty")
	}
}

func TestDeleteInitialUser(t *testing.T) {
	logf.SetLogger(zap.LoggerTo(os.Stdout, true))

	scheme := scheme.Scheme
	orgv1.SchemeBuilder.AddToScheme(scheme)
	scheme.AddKnownTypes(userv1.SchemeGroupVersion, &userv1.UserList{}, &userv1.User{})
	scheme.AddKnownTypes(oauth_config.SchemeGroupVersion, &oauth_config.OAuth{})
	scheme.AddKnownTypes(userv1.SchemeGroupVersion, &userv1.Identity{})
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	scheme.AddKnownTypes(userv1.SchemeGroupVersion, &userv1.User{})

	oAuth := &oauth_config.OAuth{
		ObjectMeta: v1.ObjectMeta{
			Name: "cluster",
		},
		Spec:       oauth_config.OAuthSpec{IdentityProviders: []oauth_config.IdentityProvider{*newHtpasswdProvider()}},
	}
	cheSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      initialUserSecret,
			Namespace: testNamespace,
		},
	}
	htpasswdSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      htpasswdSecretName,
			Namespace: ocConfigNamespace,
		},
	}
	userIdentity := &userv1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name: htpasswdIdentityProviderName + ":" + initialUserName,
		},
	}
	user := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: initialUserName,
		},
	}

	runtimeClient := fake.NewFakeClientWithScheme(scheme, oAuth, cheSecret, htpasswdSecret, userIdentity, user)

	initialUserHandler := &InitialUserOperatorHandler{
		runtimeClient: runtimeClient,
	}

	if err := initialUserHandler.DeleteOauthInitialUser(testNamespace); err != nil {
		t.Errorf("Unable to delete initial user: %s", err.Error())
	}

	expectedCheSecret := &corev1.Secret{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: initialUserSecret, Namespace: testNamespace}, expectedCheSecret); !errors.IsNotFound(err) {
		t.Errorf("Initial user secret be deleted")
	}

	expectedHtpasswsSecret := &corev1.Secret{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: htpasswdSecretName, Namespace: ocConfigNamespace}, expectedHtpasswsSecret); !errors.IsNotFound(err) {
		t.Errorf("Initial user secret should be deleted")
	}

	expectedUserIdentity := &userv1.Identity{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: htpasswdIdentityProviderName + ":" + initialUserName}, expectedUserIdentity); !errors.IsNotFound(err) {
		t.Errorf("Initial user identity should be deleted")
	}

	expectedUser := &userv1.User{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: initialUserName}, expectedUser); !errors.IsNotFound(err) {
		t.Errorf("Initial user should be deleted")
	}

	expectedOAuth := &oauth_config.OAuth{}
	if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, expectedOAuth); err != nil {
		t.Errorf("OAuth should exists")
	}

	if len(expectedOAuth.Spec.IdentityProviders) != 0 {
		t.Error("List identity providers should be an empty")
	}
}
