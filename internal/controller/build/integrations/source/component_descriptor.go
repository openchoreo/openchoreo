/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package descriptor

import choreov1 "github.com/choreo-idp/choreo/api/v1"

// Config represents the root configuration structure
type Config struct {
	SchemaVersion string     `yaml:"schemaVersion"`
	Endpoints     []Endpoint `yaml:"endpoints"`
}

// Endpoint represents an individual service endpoint configuration
type Endpoint struct {
	Name                string                       `yaml:"name"`
	DisplayName         string                       `yaml:"displayName,omitempty"`
	Service             Service                      `yaml:"service"`
	NetworkVisibilities []choreov1.NetworkVisibility `yaml:"networkVisibilities,omitempty"`
	Type                choreov1.EndpointType        `yaml:"type"`
}

// Service contains the service-specific configuration
type Service struct {
	BasePath string `yaml:"basePath,omitempty"`
	Port     int32  `yaml:"port"`
}
