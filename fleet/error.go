// Copyright (c) 2016 Pulcy.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fleet

import (
	"github.com/juju/errgo"
)

var (
	NotFoundError = errgo.New("not found")
	TimeoutError  = errgo.New("timeout")
	maskAny       = errgo.MaskFunc(errgo.Any)
)

func IsNotFound(err error) bool {
	return errgo.Cause(err) == NotFoundError
}

func IsTimeout(err error) bool {
	return errgo.Cause(err) == TimeoutError
}
