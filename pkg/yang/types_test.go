// Copyright 2015 Google Inc.
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

package yang

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/openconfig/gnmi/errdiff"
)

func TestTypeResolve(t *testing.T) {
	for x, tt := range []struct {
		in  *Type
		err string
		out *YangType
	}{
		{
			in: &Type{
				Name: "int64",
			},
			out: &YangType{
				Name:  "int64",
				Kind:  Yint64,
				Range: Int64Range,
			},
		},
		{
			in: &Type{
				Name:  "int8",
				Range: &Range{Name: "-10 .. 128"},
			},
			err: "unknown: bad range: -10..128 not within -128..127",
		},
		{
			in: &Type{
				Name:           "boolean",
				FractionDigits: &Value{Name: "42"},
			},
			err: "unknown: fraction-digits only allowed for decimal64 values",
		},
		{
			in: &Type{
				Name: "decimal64",
			},
			err: "unknown: value is required in the range of [1..18]",
		},
		{
			in: &Type{
				Name: "identityref",
			},
			err: "unknown: an identityref must specify a base",
		},
		{
			in: &Type{
				Name:           "decimal64",
				FractionDigits: &Value{Name: "42"},
			},
			err: "unknown: value 42 out of range [1..18]",
		},
		{
			in: &Type{
				Name:           "decimal64",
				FractionDigits: &Value{Name: "42"},
				Range:          &Range{Name: "-10 .. 10"},
			},
			err: "unknown: value 42 out of range [1..18]",
		},
		{
			in: &Type{
				Name:           "decimal64",
				FractionDigits: &Value{Name: "7"},
			},
			out: &YangType{
				Name:           "decimal64",
				Kind:           Ydecimal64,
				FractionDigits: 7,
				Range:          Decimal64Range,
			},
		},
		// TODO(borman): Add in more tests as we honor more fields
		// in Type.
	} {
		// We can initialize a value to ourself, so to it here.
		errs := tt.in.resolve()

		// TODO(borman):  Do not hack out Root and Base.  These
		// are hacked out for now because they can be self-referential,
		// making construction of them difficult.
		tt.in.YangType.Root = nil
		tt.in.YangType.Base = nil

		switch {
		case tt.err == "" && len(errs) > 0:
			t.Errorf("#%d: unexpected errors: %v", x, errs)
		case tt.err != "" && len(errs) == 0:
			t.Errorf("#%d: did not get expected errors: %v", x, tt.err)
		case len(errs) > 1:
			t.Errorf("#%d: too many errors: %v", x, errs)
		case len(errs) == 1 && errs[0].Error() != tt.err:
			t.Errorf("#%d: got error %v, want %s", x, errs[0], tt.err)
		case len(errs) != 0:
		case !reflect.DeepEqual(tt.in.YangType, tt.out):
			t.Errorf("#%d: got %#v, want %#v", x, tt.in.YangType, tt.out)
		}
	}
}

func TestPattern(t *testing.T) {
	tests := []struct {
		desc                string
		inGetFn             func(*Modules) (*YangType, error)
		leafNode            string
		wantPatternsRegular []string
		wantPatternsPOSIX   []string
		wantErrSubstr       string
	}{{
		desc: "Only normal patterns",
		leafNode: `
			leaf test-leaf {
				type string {
					o:bar 'coo';
					o:bar 'foo';
					pattern 'charlie';
					o:bar 'goo';
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantPatternsRegular: []string{"charlie"},
	}, {
		desc: "Only posix patterns",
		leafNode: `
			leaf test-leaf {
				type string {
					o:bar 'coo';
					o:posix-pattern 'bravo';
					o:bar 'foo';
					o:posix-pattern 'charlie';
					o:bar 'goo';
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantPatternsPOSIX: []string{"bravo", "charlie"},
	}, {
		desc: "No patterns",
		leafNode: `
			leaf test-leaf {
				type string;
			}
		}`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantPatternsRegular: nil,
		wantPatternsPOSIX:   nil,
	}, {
		desc: "Both patterns",
		leafNode: `
			leaf test-leaf {
				type string {
					pattern 'alpha';
					o:posix-pattern 'bravo';
					o:posix-pattern 'charlie';
					o:bar 'coo';
					o:posix-pattern 'delta';
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantPatternsRegular: []string{"alpha"},
		wantPatternsPOSIX:   []string{"bravo", "charlie", "delta"},
	}, {
		desc: "Both patterns, but with non-openconfig-extensions pretenders",
		leafNode: `
			leaf test-leaf {
				type string {
					pattern 'alpha';
					o:bar 'coo';
					o:posix-pattern 'delta';

					n:posix-pattern 'golf';

					pattern 'bravo';
					o:bar 'foo';
					o:posix-pattern 'echo';

					pattern 'charlie';
					o:bar 'goo';
					o:posix-pattern 'foxtrot';

					n:posix-pattern 'hotel';
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantPatternsRegular: []string{"alpha", "bravo", "charlie"},
		wantPatternsPOSIX:   []string{"delta", "echo", "foxtrot"},
	}, {
		desc: "Union type",
		leafNode: `
			leaf test-leaf {
				type union {
					type string {
						pattern 'alpha';
						o:bar 'coo';
						o:posix-pattern 'delta';

						pattern 'bravo';
						o:bar 'foo';
						o:posix-pattern 'echo';
						n:posix-pattern 'echo2';

						pattern 'charlie';
						o:bar 'goo';
						o:posix-pattern 'foxtrot';
					}
					type uint64;
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type.Type[0], nil
		},
		wantPatternsRegular: []string{"alpha", "bravo", "charlie"},
		wantPatternsPOSIX:   []string{"delta", "echo", "foxtrot"},
	}, {
		desc: "typedef",
		leafNode: `
			leaf test-leaf {
				type leaf-type;
			}

			typedef leaf-type {
				type string {
					pattern 'alpha';
					o:bar 'coo';
					o:posix-pattern 'delta';

					pattern 'bravo';
					o:bar 'foo';
					o:posix-pattern 'echo';

					pattern 'charlie';
					o:bar 'goo';
					o:posix-pattern 'foxtrot';
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantPatternsRegular: []string{"alpha", "bravo", "charlie"},
		wantPatternsPOSIX:   []string{"delta", "echo", "foxtrot"},
	}, {
		desc: "invalid POSIX pattern",
		leafNode: `
			leaf test-leaf {
				type leaf-type;
			}

			typedef leaf-type {
				type string {
					o:posix-pattern '?';
				}
			}
		} // end module`,
		inGetFn: func(ms *Modules) (*YangType, error) {
			m, err := ms.FindModuleByPrefix("t")
			if err != nil {
				return nil, fmt.Errorf("can't find module in %v", ms)
			}
			if len(m.Leaf) == 0 {
				return nil, fmt.Errorf("node %v is missing imports", m)
			}
			e := ToEntry(m)
			return e.Dir["test-leaf"].Type, nil
		},
		wantErrSubstr: "bad pattern",
	}}

	for _, tt := range tests {
		inModules := map[string]string{
			"test": `
				module test {
					prefix "t";
					namespace "urn:t";

					import non-openconfig-extensions {
						prefix "n";
						description "non-openconfig-extensions module";
					}
					import openconfig-extensions {
						prefix "o";
						description "openconfig-extensions module";
					}` + tt.leafNode,
			"openconfig-extensions": `
				module openconfig-extensions {
					prefix "o";
					namespace "urn:o";

					extension bar {
						argument "baz";
					}

					extension posix-pattern {
						argument "pattern";
					}
				}
			`,
			"non-openconfig-extensions": `
				module non-openconfig-extensions {
					prefix "n";
					namespace "urn:n";

					extension bar {
						argument "baz";
					}

					extension posix-pattern {
						argument "pattern";
					}
				}
			`,
		}

		t.Run(tt.desc, func(t *testing.T) {
			ms := NewModules()
			for n, m := range inModules {
				if err := ms.Parse(m, n); err != nil {
					t.Fatalf("error parsing module %s, got: %v, want: nil", n, err)
				}
			}
			errs := ms.Process()
			var err error
			if len(errs) > 1 {
				t.Fatalf("Got more than 1 error: %v", errs)
			} else if len(errs) == 1 {
				err = errs[0]
			}
			if diff := errdiff.Substring(err, tt.wantErrSubstr); diff != "" {
				t.Errorf("Did not get expected error: %s", diff)
			}
			if err != nil {
				return
			}

			yangType, err := tt.inGetFn(ms)
			if err != nil {
				t.Fatal(err)
			}

			sort.Strings(yangType.Pattern)
			sort.Strings(tt.wantPatternsRegular)
			if diff := cmp.Diff(yangType.Pattern, tt.wantPatternsRegular, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Type.resolve() pattern test (-got, +want):\n%s", diff)
			}

			sort.Strings(yangType.POSIXPattern)
			sort.Strings(tt.wantPatternsPOSIX)
			if diff := cmp.Diff(yangType.POSIXPattern, tt.wantPatternsPOSIX, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Type.resolve() posix-pattern test (-got, +want):\n%s", diff)
			}
		})
	}
}
