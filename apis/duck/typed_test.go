/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package duck_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	"knative.dev/pkg/apis/duck"
	duckv1alpha1 "knative.dev/pkg/apis/duck/v1alpha1"
	. "knative.dev/pkg/testing"
)

func TestSimpleList(t *testing.T) {
	scheme := runtime.NewScheme()
	AddToScheme(scheme)
	duckv1alpha1.AddToScheme(scheme)

	namespace, name, want := "foo", "bar", "my_hostname"

	// Despite the signature allowing `...runtime.Object`, this method
	// will not work properly unless the passed objects are `unstructured.Unstructured`
	client := fake.NewSimpleDynamicClient(scheme, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "pkg.knative.dev/v2",
			"kind":       "Resource",
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			},
			"status": map[string]interface{}{
				"address": map[string]interface{}{
					"hostname": want,
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tif := &duck.TypedInformerFactory{
		Client:       client,
		Type:         &duckv1alpha1.AddressableType{},
		ResyncPeriod: 1 * time.Second,
		StopChannel:  ctx.Done(),
	}

	// This hangs without:
	// https://github.com/kubernetes/kubernetes/pull/68552
	_, lister, err := tif.Get(ctx, SchemeGroupVersion.WithResource("resources"))
	if err != nil {
		t.Fatal("Get() =", err)
	}

	elt, err := lister.ByNamespace(namespace).Get(name)
	if err != nil {
		t.Fatal("Get() =", err)
	}

	got, ok := elt.(*duckv1alpha1.AddressableType)
	if !ok {
		t.Fatalf("Get() = %T, wanted *duckv1alpha1.AddressableType", elt)
	}

	if gotHostname := got.Status.Address.Hostname; gotHostname != want {
		t.Errorf("Get().Status.Address.Hostname = %v, wanted %v", gotHostname, want)
	}

	// TODO(mattmoor): Access through informer
}

func TestInvalidResource(t *testing.T) {
	client := &invalidResourceClient{}
	stopCh := make(chan struct{})
	defer close(stopCh)

	tif := &duck.TypedInformerFactory{
		Client:       client,
		Type:         &duckv1alpha1.AddressableType{},
		ResyncPeriod: 1 * time.Second,
		StopChannel:  stopCh,
	}

	_, _, got := tif.Get(context.Background(), SchemeGroupVersion.WithResource("resources"))

	if !errors.Is(got, errTest) {
		t.Errorf("Error = %v, want: %v", got, errTest)
	}
}

func TestAsStructuredWatcherNestedError(t *testing.T) {
	want := errors.New("this is what we expect")
	nwf := func(ctx context.Context, lo metav1.ListOptions) (watch.Interface, error) {
		return nil, want
	}

	ctx := context.Background()
	wf := duck.AsStructuredWatcher(nwf, &duckv1alpha1.AddressableType{})

	_, got := wf(ctx, metav1.ListOptions{})
	if !errors.Is(got, want) {
		t.Errorf("WatchFunc() = %v, wanted %v", got, want)
	}
}

func TestAsStructuredWatcherClosedChannel(t *testing.T) {
	nwf := func(ctx context.Context, lo metav1.ListOptions) (watch.Interface, error) {
		return watch.NewEmptyWatch(), nil
	}

	ctx := context.Background()
	wf := duck.AsStructuredWatcher(nwf, &duckv1alpha1.AddressableType{})

	wi, err := wf(ctx, metav1.ListOptions{})
	if err != nil {
		t.Error("WatchFunc() =", err)
	}

	ch := wi.ResultChan()

	x, ok := <-ch
	if ok {
		t.Errorf("<-ch = %v, wanted closed", x)
	}
}

func TestAsStructuredWatcherPassThru(t *testing.T) {
	unstructuredCh := make(chan watch.Event)
	nwf := func(ctx context.Context, lo metav1.ListOptions) (watch.Interface, error) {
		return watch.NewProxyWatcher(unstructuredCh), nil
	}

	ctx := context.Background()
	wf := duck.AsStructuredWatcher(nwf, &duckv1alpha1.AddressableType{})

	wi, err := wf(ctx, metav1.ListOptions{})
	if err != nil {
		t.Error("WatchFunc() =", err)
	}
	defer wi.Stop()
	ch := wi.ResultChan()

	// Don't expect a message yet.
	select {
	case x, ok := <-ch:
		t.Errorf("Saw unexpected message on channel: %v, %v.", x, ok)
	case <-time.After(100 * time.Millisecond):
		// Expected path.
	}

	want := watch.Added
	unstructuredCh <- watch.Event{
		Type:   want,
		Object: &unstructured.Unstructured{},
	}

	// Expect a message when we send one though.
	select {
	case x, ok := <-ch:
		if !ok {
			t.Fatal("<-ch = closed, wanted *duckv1alpha1.AddressableType{}")
		}
		if got := x.Type; got != want {
			t.Errorf("x.Type = %v, wanted %v", got, want)
		}
		if _, ok := x.Object.(*duckv1alpha1.AddressableType); !ok {
			t.Errorf("<-ch = %T, wanted %T", x, &duckv1alpha1.AddressableType{})
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Didn't see expected message on channel.")
	}
}

func TestAsStructuredWatcherPassThruErrors(t *testing.T) {
	unstructuredCh := make(chan watch.Event)
	nwf := func(ctx context.Context, lo metav1.ListOptions) (watch.Interface, error) {
		return watch.NewProxyWatcher(unstructuredCh), nil
	}

	ctx := context.Background()
	wf := duck.AsStructuredWatcher(nwf, &duckv1alpha1.AddressableType{})

	wi, err := wf(ctx, metav1.ListOptions{})
	if err != nil {
		t.Error("WatchFunc() =", err)
	}
	defer wi.Stop()
	ch := wi.ResultChan()

	want := watch.Event{
		Type: watch.Error,
		Object: &metav1.Status{
			Code: 42,
		},
	}
	unstructuredCh <- want

	// Expect a message when we send one though.
	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatal("<-ch = closed, wanted *metav1.Status{}")
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Error("<-ch (-want, +got) =", diff)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Didn't see expected message on channel.")
	}
}

func TestAsStructuredWatcherErrorConverting(t *testing.T) {
	unstructuredCh := make(chan watch.Event)
	nwf := func(ctx context.Context, lo metav1.ListOptions) (watch.Interface, error) {
		return watch.NewProxyWatcher(unstructuredCh), nil
	}

	ctx := context.Background()
	wf := duck.AsStructuredWatcher(nwf, &badObject{})

	wi, err := wf(ctx, metav1.ListOptions{})
	if err != nil {
		t.Error("WatchFunc() =", err)
	}
	defer wi.Stop()
	ch := wi.ResultChan()

	unstructuredCh <- watch.Event{
		Type: watch.Added,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	// Expect a message when we send one though.
	select {
	case x, ok := <-ch:
		if !ok {
			t.Fatal("<-ch = closed, wanted *duckv1alpha1.Generational{}")
		}
		if got, want := x.Type, watch.Error; got != want {
			t.Errorf("<-ch = %v, wanted %v", got, want)
		}
		if status, ok := x.Object.(*metav1.Status); !ok {
			t.Errorf("<-ch = %T, wanted %T", x, &metav1.Status{})
		} else if got, want := status.Message, errNoUnmarshal.Error(); got != want {
			t.Errorf("<-ch = %v, wanted %v", got, want)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Didn't see expected message on channel.")
	}
}

var errNoUnmarshal = errors.New("this cannot be unmarshalled")

type badObject struct {
	Foo doNotUnmarshal `json:"foo"`
}

type doNotUnmarshal struct{}

var _ json.Unmarshaler = (*doNotUnmarshal)(nil)

func (*doNotUnmarshal) UnmarshalJSON([]byte) error {
	return errNoUnmarshal
}

func (bo *badObject) GetObjectKind() schema.ObjectKind {
	return &metav1.TypeMeta{}
}

func (bo *badObject) DeepCopyObject() runtime.Object {
	return &badObject{}
}

var errTest = errors.New("failed to get list")

type invalidResourceClient struct {
	*fake.FakeDynamicClient
}

func (*invalidResourceClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &invalidResource{}
}

type invalidResource struct {
	dynamic.NamespaceableResourceInterface
}

func (*invalidResource) List(ctx context.Context, options metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, errTest
}
