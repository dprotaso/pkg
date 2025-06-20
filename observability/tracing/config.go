/*
Copyright 2025 The Knative Authors

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

package tracing

import (
	configmap "knative.dev/pkg/configmap/parser"
)

const (
	ProtocolGRPC         = "grpc"
	ProtocolHTTPProtobuf = "http/protobuf"
	ProtocolNone         = "none"
)

type Config struct {
	Protocol     string
	Endpoint     string
	SamplingRate float64
}

func DefaultConfig() Config {
	return Config{
		Protocol: ProtocolNone,
	}
}

func NewFromMap(m map[string]string) (Config, error) {
	c := DefaultConfig()

	err := configmap.Parse(m,
		configmap.As("tracing-protocol", &c.Protocol),
		configmap.As("tracing-endpoint", &c.Endpoint),
		configmap.As("tracing-export-interval", &c.Endpoint),
	)

	return c, err
}
