/*
Copyright 2019 The Kubernetes Authors.

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

package minibroker

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

const mariadbProtocolName = "mysql"

type MariadbProvider struct{}

func (p MariadbProvider) Bind(services []corev1.Service, params map[string]interface{}, chartSecrets map[string]interface{}) (*Credentials, error) {
	service := services[0]
	if len(service.Spec.Ports) == 0 {
		return nil, errors.Errorf("no ports found")
	}
	svcPort := service.Spec.Ports[0]

	host := buildHostFromService(service)

	dbParams, ok := params["db"].(map[string]interface{})
	if !ok {
		dbParams = make(map[string]interface{})
	}

	database := ""
	dbVal, ok := dbParams["name"]
	if ok {
		database, ok = dbVal.(string)
		if !ok {
			return nil, errors.Errorf("db.name not a string")
		}
	}

	var user, password string
	userVal, ok := dbParams["user"]
	if ok {
		user, ok = userVal.(string)
		if !ok {
			return nil, errors.Errorf("db.user not a string")
		}

		passwordVal, ok := chartSecrets["mariadb-password"]
		if !ok {
			return nil, errors.Errorf("mariadb-password not found in secret keys")
		}
		password, ok = passwordVal.(string)
		if !ok {
			return nil, errors.Errorf("password not a string")
		}
	} else {
		user = "root"

		rootPassword, ok := chartSecrets["mariadb-root-password"]
		if !ok {
			return nil, errors.Errorf("mariadb-root-password not found in secret keys")
		}
		password, ok = rootPassword.(string)
		if !ok {
			return nil, errors.Errorf("password not a string")
		}
	}

	creds := Credentials{
		Protocol: mariadbProtocolName,
		Port:     svcPort.Port,
		Host:     host,
		Username: user,
		Password: password,
		Database: database,
	}
	creds.URI = buildURI(creds)

	return &creds, nil
}
