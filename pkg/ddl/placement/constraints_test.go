// Copyright 2021 PingCAP, Inc.
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

package placement

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	pd "github.com/tikv/pd/client/http"
)

func TestNewConstraints(t *testing.T) {
	_, err := NewConstraints(nil)
	require.NoError(t, err)

	_, err = NewConstraints([]string{})
	require.NoError(t, err)

	_, err = NewConstraints([]string{"+zonesh"})
	require.ErrorIs(t, err, ErrInvalidConstraintFormat)

	_, err = NewConstraints([]string{"+zone=sh", "-zone=sh"})
	require.ErrorIs(t, err, ErrConflictingConstraints)
}

func TestAdd(t *testing.T) {
	type TestCase struct {
		name   string
		labels []pd.LabelConstraint
		label  pd.LabelConstraint
		err    error
	}
	var tests []TestCase

	labels, err := NewConstraints([]string{"+zone=sh"})
	require.NoError(t, err)
	label, err := NewConstraint("-zone=sh")
	require.NoError(t, err)
	tests = append(tests, TestCase{
		"always false match",
		labels, label,
		ErrConflictingConstraints,
	})

	labels, err = NewConstraints([]string{"+zone=sh"})
	require.NoError(t, err)
	label, err = NewConstraint("+zone=sh")
	require.NoError(t, err)
	tests = append(tests, TestCase{
		"duplicated constraints, skip",
		labels, label,
		nil,
	})

	tests = append(tests, TestCase{
		"duplicated constraints should not stop conflicting constraints check",
		append(labels, pd.LabelConstraint{
			Op:     pd.NotIn,
			Key:    "zone",
			Values: []string{"sh"},
		}), label,
		ErrConflictingConstraints,
	})

	labels, err = NewConstraints([]string{"+zone=sh"})
	require.NoError(t, err)
	tests = append(tests, TestCase{
		"invalid label in operand",
		labels, pd.LabelConstraint{Op: "["},
		nil,
	})

	tests = append(tests, TestCase{
		"invalid label in operator",
		[]pd.LabelConstraint{{Op: "["}}, label,
		nil,
	})

	tests = append(tests, TestCase{
		"invalid label in both, same key",
		[]pd.LabelConstraint{{Op: "[", Key: "dc"}}, pd.LabelConstraint{Op: "]", Key: "dc"},
		ErrConflictingConstraints,
	})

	labels, err = NewConstraints([]string{"+zone=sh"})
	require.NoError(t, err)
	label, err = NewConstraint("-zone=bj")
	require.NoError(t, err)
	tests = append(tests, TestCase{
		"normal",
		labels, label,
		nil,
	})

	for _, test := range tests {
		err := AddConstraint(&test.labels, test.label)
		comment := fmt.Sprintf("%s: %v", test.name, err)
		if test.err == nil {
			require.NoError(t, err, comment)
			require.Equal(t, test.label, test.labels[len(test.labels)-1], comment)
		} else {
			require.ErrorIs(t, err, test.err, comment)
		}
	}
}

func TestRestoreConstraints(t *testing.T) {
	type TestCase struct {
		name   string
		input  []pd.LabelConstraint
		output string
		err    error
	}
	var tests []TestCase

	tests = append(tests, TestCase{
		"normal1",
		[]pd.LabelConstraint{},
		"",
		nil,
	})

	input1, err := NewConstraint("+zone=bj")
	require.NoError(t, err)
	input2, err := NewConstraint("-zone=sh")
	require.NoError(t, err)
	tests = append(tests, TestCase{
		"normal2",
		[]pd.LabelConstraint{input1, input2},
		`"+zone=bj","-zone=sh"`,
		nil,
	})

	tests = append(tests, TestCase{
		"error",
		[]pd.LabelConstraint{{
			Op:     "[",
			Key:    "dc",
			Values: []string{"dc1"},
		}},
		"",
		ErrInvalidConstraintFormat,
	})

	for _, test := range tests {
		res, err := RestoreConstraints(&test.input)
		comment := fmt.Sprintf("%s: %v", test.name, err)
		if test.err == nil {
			require.NoError(t, err, comment)
			require.Equal(t, test.output, res, comment)
		} else {
			require.ErrorIs(t, err, test.err, comment)
		}
	}
}
