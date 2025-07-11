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

package duck

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func TestEnqueueInformerFactory(t *testing.T) {
	called := false
	want := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			called = true
		},
	}
	fsii := &fakeSharedIndexInformer{t: t}
	fif := &FixedInformerFactory{inf: fsii}
	eif := &EnqueueInformerFactory{
		Delegate:     fif,
		EventHandler: want,
	}

	gvr := schema.GroupVersionResource{
		Group:    "testing.knative.dev",
		Version:  "v3",
		Resource: "caches",
	}
	inf, _, err := eif.Get(context.Background(), gvr)
	if err != nil {
		t.Fatal("Get() =", err)
	}
	if inf != fsii {
		t.Fatalf("Get() = %v, wanted %v", inf, fsii)
	}

	got, ok := fsii.eventHandler.(cache.ResourceEventHandlerFuncs)
	if !ok {
		t.Errorf("eventHandler = %T, wanted %T", fsii.eventHandler, want)
	}
	if called {
		t.Error("Want not called, got called")
	}

	got.AddFunc(nil)

	if !called {
		t.Error("Want called, got not called")
	}

	if got.UpdateFunc != nil {
		t.Error("UpdateFunc = non-nil, wanted nil")
	}

	if got.DeleteFunc != nil {
		t.Error("DeleteFunc = non-nil, wanted nil")
	}
}

func TestEnqueueInformerFactoryWithFailure(t *testing.T) {
	want := errors.New("expected error")
	fif := &FixedInformerFactory{err: want}
	eif := &EnqueueInformerFactory{
		Delegate: fif,
		EventHandler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				t.Error("Unexpected call to AddFunc.")
			},
			UpdateFunc: func(old, new interface{}) {
				t.Error("Unexpected call to UpdateFunc.")
			},
		},
	}

	gvr := schema.GroupVersionResource{
		Group:    "testing.knative.dev",
		Version:  "v3",
		Resource: "caches",
	}
	inf, _, got := eif.Get(context.Background(), gvr)
	if !errors.Is(got, want) {
		t.Fatalf("Get() = %v, wanted %v", got, want)
	}

	if inf != nil {
		t.Fatal("Get() = non nil, wanted nil")
	}
}

type FixedInformerFactory struct {
	inf    cache.SharedIndexInformer
	lister cache.GenericLister
	err    error
}

var _ InformerFactory = (*FixedInformerFactory)(nil)

func (fif *FixedInformerFactory) Get(ctx context.Context, gvr schema.GroupVersionResource) (cache.SharedIndexInformer, cache.GenericLister, error) {
	return fif.inf, fif.lister, fif.err
}

type fakeSharedIndexInformer struct {
	t            *testing.T
	eventHandler cache.ResourceEventHandler
}

var _ cache.SharedIndexInformer = (*fakeSharedIndexInformer)(nil)

func (fsii *fakeSharedIndexInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	fsii.eventHandler = handler
	return nil, nil
}

func (fsii *fakeSharedIndexInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	fsii.t.Fatalf("NYI: AddEventHandlerWithResyncPeriod")
	return nil, nil
}

func (fsii *fakeSharedIndexInformer) AddEventHandlerWithOptions(handler cache.ResourceEventHandler, options cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	fsii.t.Fatalf("NYI: AddEventHandlerWithOptions")
	return nil, nil
}

func (fsii *fakeSharedIndexInformer) GetStore() cache.Store {
	fsii.t.Fatalf("NYI: GetStore")
	return nil
}

func (fsii *fakeSharedIndexInformer) GetController() cache.Controller {
	fsii.t.Fatalf("NYI: GetController")
	return nil
}

func (fsii *fakeSharedIndexInformer) Run(stopCh <-chan struct{}) {
	fsii.t.Fatalf("NYI: Run")
}

func (fsii *fakeSharedIndexInformer) RunWithContext(ctx context.Context) {
	fsii.t.Fatalf("NYI: RunWithContext")
}

func (fsii *fakeSharedIndexInformer) HasSynced() bool {
	fsii.t.Fatalf("NYI: HadSynced")
	return false
}

func (fsii *fakeSharedIndexInformer) LastSyncResourceVersion() string {
	fsii.t.Fatalf("NYI: LastSyncResourceVersion")
	return ""
}

func (fsii *fakeSharedIndexInformer) AddIndexers(indexers cache.Indexers) error {
	fsii.t.Fatalf("NYI: AddIndexers")
	return nil
}

func (fsii *fakeSharedIndexInformer) GetIndexer() cache.Indexer {
	fsii.t.Fatalf("NYI: GetIndexer")
	return nil
}

func (fsii *fakeSharedIndexInformer) SetWatchErrorHandler(handler cache.WatchErrorHandler) error {
	fsii.t.Fatalf("NYI: SetWatchErrorHandler")
	return nil
}

func (fsii *fakeSharedIndexInformer) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	fsii.t.Fatalf("NYI: SetWatchErrorHandler")
	return nil
}

func (fsii *fakeSharedIndexInformer) SetTransform(handler cache.TransformFunc) error {
	fsii.t.Fatalf("NYI: SetTransform")
	return nil
}

func (fsii *fakeSharedIndexInformer) IsStopped() bool {
	fsii.t.Fatalf("NYI: IsStopped")
	return false
}

func (fsii *fakeSharedIndexInformer) RemoveEventHandler(handler cache.ResourceEventHandlerRegistration) error {
	fsii.t.Fatalf("NYI: RemoveEventHandler")
	return nil
}
