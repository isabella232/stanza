package transformer

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/observiq/carbon/entry"
	"github.com/observiq/carbon/internal/testutil"
	"github.com/observiq/carbon/plugin"
	"github.com/observiq/carbon/plugin/helper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"
)

func NewFakeRestructurePlugin() (*RestructurePlugin, *testutil.Plugin) {
	mock := testutil.Plugin{}
	logger, _ := zap.NewProduction()
	return &RestructurePlugin{
		TransformerPlugin: helper.TransformerPlugin{
			BasicPlugin: helper.BasicPlugin{
				PluginID:      "test",
				PluginType:    "restructure",
				SugaredLogger: logger.Sugar(),
			},
			WriterPlugin: helper.WriterPlugin{
				OutputPlugins: []plugin.Plugin{&mock},
			},
		},
	}, &mock
}

func TestRestructurePlugin(t *testing.T) {
	os.Setenv("TEST_RESTRUCTURE_PLUGIN_ENV", "foo")
	defer os.Unsetenv("TEST_RESTRUCTURE_PLUGIN_ENV")

	newTestEntry := func() *entry.Entry {
		e := entry.New()
		e.Timestamp = time.Unix(1586632809, 0)
		e.Record = map[string]interface{}{
			"key": "val",
			"nested": map[string]interface{}{
				"nestedkey": "nestedval",
			},
		}
		return e
	}

	cases := []struct {
		name   string
		ops    []Op
		input  *entry.Entry
		output *entry.Entry
	}{
		{
			name:   "Nothing",
			input:  newTestEntry(),
			output: newTestEntry(),
		},
		{
			name: "AddValue",
			ops: []Op{
				{
					&OpAdd{
						Field: entry.NewRecordField("new"),
						Value: "message",
					},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record.(map[string]interface{})["new"] = "message"
				return e
			}(),
		},
		{
			name: "AddValueExpr",
			ops: []Op{
				{
					&OpAdd{
						Field: entry.NewRecordField("new"),
						program: func() *vm.Program {
							vm, err := expr.Compile(`$.key + "_suffix"`)
							require.NoError(t, err)
							return vm
						}(),
					},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record.(map[string]interface{})["new"] = "val_suffix"
				return e
			}(),
		},
		{
			name: "AddValueExprEnv",
			ops: []Op{
				{
					&OpAdd{
						Field: entry.NewRecordField("new"),
						program: func() *vm.Program {
							vm, err := expr.Compile(`env("TEST_RESTRUCTURE_PLUGIN_ENV")`)
							require.NoError(t, err)
							return vm
						}(),
					},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record.(map[string]interface{})["new"] = "foo"
				return e
			}(),
		},
		{
			name: "Remove",
			ops: []Op{
				{
					&OpRemove{entry.NewRecordField("nested")},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record = map[string]interface{}{
					"key": "val",
				}
				return e
			}(),
		},
		{
			name: "Retain",
			ops: []Op{
				{
					&OpRetain{[]entry.Field{entry.NewRecordField("key")}},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record = map[string]interface{}{
					"key": "val",
				}
				return e
			}(),
		},
		{
			name: "Move",
			ops: []Op{
				{
					&OpMove{
						From: entry.NewRecordField("key"),
						To:   entry.NewRecordField("newkey"),
					},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record = map[string]interface{}{
					"newkey": "val",
					"nested": map[string]interface{}{
						"nestedkey": "nestedval",
					},
				}
				return e
			}(),
		},
		{
			name: "Flatten",
			ops: []Op{
				{
					&OpFlatten{
						Field: entry.RecordField{
							Keys: []string{"nested"},
						},
					},
				},
			},
			input: newTestEntry(),
			output: func() *entry.Entry {
				e := newTestEntry()
				e.Record = map[string]interface{}{
					"key":       "val",
					"nestedkey": "nestedval",
				}
				return e
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			plugin, mockOutput := NewFakeRestructurePlugin()
			plugin.ops = tc.ops

			mockOutput.On("Process", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				require.Equal(t, tc.output, args[1].(*entry.Entry))
			}).Return(nil)

			err := plugin.Process(context.Background(), tc.input)
			require.NoError(t, err)
		})
	}
}

func TestRestructureSerializeRoundtrip(t *testing.T) {
	cases := []struct {
		name string
		op   Op
	}{
		{
			name: "AddValue",
			op: Op{&OpAdd{
				Field: entry.NewRecordField("new"),
				Value: "message",
			}},
		},
		{
			name: "AddValueExpr",
			op: Op{&OpAdd{
				Field: entry.NewRecordField("new"),
				ValueExpr: func() *string {
					s := `$.key + "_suffix"`
					return &s
				}(),
				program: func() *vm.Program {
					vm, err := expr.Compile(`$.key + "_suffix"`)
					require.NoError(t, err)
					return vm
				}(),
			}},
		},
		{
			name: "Remove",
			op:   Op{&OpRemove{entry.NewRecordField("nested")}},
		},
		{
			name: "Retain",
			op:   Op{&OpRetain{[]entry.Field{entry.NewRecordField("key")}}},
		},
		{
			name: "Move",
			op: Op{&OpMove{
				From: entry.NewRecordField("key"),
				To:   entry.NewRecordField("newkey"),
			}},
		},
		{
			name: "Flatten",
			op: Op{&OpFlatten{
				Field: entry.RecordField{
					Keys: []string{"nested"},
				},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tc.op)
			require.NoError(t, err)

			var jsonOp Op
			err = json.Unmarshal(jsonBytes, &jsonOp)
			require.NoError(t, err)

			require.Equal(t, tc.op, jsonOp)

			yamlBytes, err := yaml.Marshal(tc.op)
			require.NoError(t, err)

			var yamlOp Op
			err = yaml.UnmarshalStrict(yamlBytes, &yamlOp)
			require.NoError(t, err)

			require.Equal(t, tc.op, yamlOp)
		})
	}
}

func TestUnmarshalAll(t *testing.T) {
	configYAML := `
type: restructure
id: my_restructure
output: test_output
ops:
  - add:
      field: "message"
      value: "val"
  - add:
      field: "message_suffix"
      value_expr: "$.message + \"_suffix\""
  - remove: "message"
  - retain:
      - "message_retain"
  - flatten: "message_flatten"
  - move:
      from: "message1"
      to: "message2"
`

	configJSON := `
{
  "type": "restructure",
  "id": "my_restructure",
  "output": "test_output",
  "ops": [{
    "add": {
      "field": "message",
      "value": "val"
    }
  },{
    "add": {
      "field": "message_suffix",
      "value_expr": "$.message + \"_suffix\""
    }
  },{
    "remove": "message"
  },{
    "retain": [
      "message_retain"
    ]
  },{
    "flatten": "message_flatten"
  },{
    "move": {
      "from": "message1",
      "to": "message2"
    }
  }]
}`

	expected := plugin.Config(plugin.Config{
		Builder: &RestructurePluginConfig{
			TransformerConfig: helper.TransformerConfig{
				BasicConfig: helper.BasicConfig{
					PluginID:   "my_restructure",
					PluginType: "restructure",
				},
				WriterConfig: helper.WriterConfig{
					OutputIDs: []string{"test_output"},
				},
			},
			Ops: []Op{
				Op{&OpAdd{
					Field: entry.NewRecordField("message"),
					Value: "val",
				}},
				Op{&OpAdd{
					Field: entry.NewRecordField("message_suffix"),
					ValueExpr: func() *string {
						s := `$.message + "_suffix"`
						return &s
					}(),
					program: func() *vm.Program {
						vm, err := expr.Compile(`$.message + "_suffix"`)
						require.NoError(t, err)
						return vm
					}(),
				}},
				Op{&OpRemove{
					Field: entry.NewRecordField("message"),
				}},
				Op{&OpRetain{
					Fields: []entry.Field{
						entry.NewRecordField("message_retain"),
					},
				}},
				Op{&OpFlatten{
					Field: entry.RecordField{
						Keys: []string{"message_flatten"},
					},
				}},
				Op{&OpMove{
					From: entry.NewRecordField("message1"),
					To:   entry.NewRecordField("message2"),
				}},
			},
		},
	})

	var unmarshalledYAML plugin.Config
	err := yaml.UnmarshalStrict([]byte(configYAML), &unmarshalledYAML)
	require.NoError(t, err)
	require.Equal(t, expected, unmarshalledYAML)

	var unmarshalledJSON plugin.Config
	err = json.Unmarshal([]byte(configJSON), &unmarshalledJSON)
	require.NoError(t, err)
	require.Equal(t, expected, unmarshalledJSON)
}

func TestOpType(t *testing.T) {
	cases := []struct {
		op           OpApplier
		expectedType string
	}{
		{
			&OpAdd{},
			"add",
		},
		{
			&OpRemove{},
			"remove",
		},
		{
			&OpRetain{},
			"retain",
		},
		{
			&OpMove{},
			"move",
		},
		{
			&OpFlatten{},
			"flatten",
		},
	}

	for _, tc := range cases {
		t.Run(tc.expectedType, func(t *testing.T) {
			require.Equal(t, tc.expectedType, tc.op.Type())
		})
	}

	t.Run("InvalidOpType", func(t *testing.T) {
		raw := "- unknown: test"
		var ops []Op
		err := yaml.UnmarshalStrict([]byte(raw), &ops)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown op type")
	})
}
