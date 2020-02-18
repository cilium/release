// Copyright 2020 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package persistence

import (
	"encoding/json"
	"io/ioutil"

	"github.com/cilium/release/pkg/types"
)

type State struct {
	BackportPRs  types.BackportPRs
	PullRequests types.PullRequests
	SHAs         []string
}

func StoreState(file string, backportPRs types.BackportPRs, prs types.PullRequests, shas []string) error {
	s := State{
		BackportPRs:  backportPRs,
		PullRequests: prs,
		SHAs:         shas,
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(file, data, 0664)
}

func LoadState(file string) (types.BackportPRs, types.PullRequests, []string, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, nil, nil, err
	}
	s := State{}
	err = json.Unmarshal(data, &s)
	if err != nil {
		return nil, nil, nil, err
	}

	return s.BackportPRs, s.PullRequests, s.SHAs, nil
}
