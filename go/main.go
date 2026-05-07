package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

var (
	structRequired = arrow.StructOf(arrow.Field{
		Name:     "child",
		Type:     arrow.PrimitiveTypes.Int8,
		Nullable: false,
	})

	structNullable = arrow.StructOf(arrow.Field{
		Name:     "child",
		Type:     arrow.PrimitiveTypes.Int8,
		Nullable: true,
	})

	schemaRequired = arrow.NewSchema([]arrow.Field{
		{Name: "parent", Type: structRequired, Nullable: true},
	}, nil)

	schemaNullable = arrow.NewSchema([]arrow.Field{
		{Name: "parent", Type: structNullable, Nullable: true},
	}, nil)
)

func tryRun(name string, fn func(memory.Allocator) error) {
	fmt.Printf("** %s **\n", name)
	mem := memory.NewGoAllocator()
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Panic: %v\n", r)
			}
		}()
		if err := fn(mem); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}()
	fmt.Println("\n---------------")
}

func main() {
	tryRun("load json with nulls in required field", loadJSONWithNullsInRequiredField)
	tryRun("create record from invalid arrays", createRecordFromInvalidArrays)
	tryRun("cast nullable into non-nullable", castToNonNullable)
	tryRun("write record with nulls into non-nullable IPC stream", writeNullableRecordIntoNonNullableIPC)
	tryRun("write valid non-nullable record to JSON", writeValidNonNullableToJSON)
	tryRun("create invalid array using struct builder", appendNullToInnerFieldUsingBuilder)
}

// buildStructArray builds a struct array under the given struct type with the
// given child values; nil pointer means "child is null".
func buildStructArray(mem memory.Allocator, dt *arrow.StructType, vals []*int8) arrow.Array {
	b := array.NewStructBuilder(mem, dt)
	defer b.Release()
	childB := b.FieldBuilder(0).(*array.Int8Builder)
	for _, v := range vals {
		b.Append(true)
		if v == nil {
			childB.AppendNull()
		} else {
			childB.Append(*v)
		}
	}
	return b.NewArray()
}

func printRecordBatch(rec arrow.RecordBatch) error {
	jsonBuf, err := rec.MarshalJSON() 
	if err != nil {
		return err
	}
	fmt.Printf("schema: %s\n", rec.Schema().String())

	for i, col := range rec.Columns() {
		name := rec.Schema().Field(i).Name
		fmt.Printf("col %s: %s\n", name, col)
	}

	fmt.Printf("MarshalJSON(): \n%s\n", jsonBuf)
	return nil
}

func loadJSONWithNullsInRequiredField(mem memory.Allocator) error {
	// arrow-go RecordFromJSON expects a JSON array of row objects,
	// not NDJSON like pyarrow.json.read_json.
	payload := `[{"parent": {"child": 1}}, {"parent": {"child": null}}]`
	rec, _, err := array.RecordFromJSON(mem, schemaRequired, strings.NewReader(payload))
	if err != nil {
		return err
	}
	defer rec.Release()
	err = printRecordBatch(rec)
	if err != nil {
		return err
	}
	return nil
}

func createRecordFromInvalidArrays(mem memory.Allocator) error {
	arr := buildStructArray(mem, structNullable, []*int8{new(int8(1)), nil})
	defer arr.Release()
	rec := array.NewRecordBatch(schemaRequired, []arrow.Array{arr}, int64(arr.Len()))
	defer rec.Release()
	err := printRecordBatch(rec)
	if err != nil {
		return err
	}
	return nil
}

func appendNullToInnerFieldUsingBuilder(mem memory.Allocator) error {
	builder := array.NewStructBuilder(mem, structRequired)
	childBuilder := builder.FieldBuilder(0).(*array.Int8Builder)
	defer builder.Release()

	builder.Append(true)
	builder.Append(true)
	childBuilder.AppendValues(
		[]int8{1, 2},
		[]bool{true, false},
	)

	arr := builder.NewArray()
	defer arr.Release()

	rec := array.NewRecordBatch(schemaRequired, []arrow.Array{arr}, int64(arr.Len()))
	defer rec.Release()
	err := printRecordBatch(rec)
	if err != nil {
		return err
	}
	return nil
}

func castToNonNullable(mem memory.Allocator) error {
	arr := buildStructArray(mem, structNullable, []*int8{new(int8(1)), nil})
	defer arr.Release()
	nullableRec := array.NewRecordBatch(schemaNullable, []arrow.Array{arr}, int64(arr.Len()))
	defer nullableRec.Release()

	// arrow-go has no Table.cast equivalent. Closest re-typing is constructing
	// a new Record with the target schema and the same column data.
	cols := nullableRec.Columns()
	requiredRec := array.NewRecordBatch(schemaRequired, cols, nullableRec.NumRows())
	defer requiredRec.Release()
	err := printRecordBatch(requiredRec)
	if err != nil {
		return err
	}
	return nil
}

func writeNullableRecordIntoNonNullableIPC(mem memory.Allocator) error {
	arr := buildStructArray(mem, structNullable, []*int8{new(int8(1)), nil})
	defer arr.Release()
	nullableRec := array.NewRecordBatch(schemaNullable, []arrow.Array{arr}, int64(arr.Len()))
	defer nullableRec.Release()

	var buf bytes.Buffer
	w, err := ipc.NewFileWriter(&buf,
		ipc.WithSchema(schemaRequired),
		ipc.WithAllocator(mem),
	)
	if err != nil {
		return fmt.Errorf("new ipc writer: %w", err)
	}
	if err := w.Write(nullableRec); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	fmt.Printf("wrote %d bytes\n", buf.Len())
	return nil
}

func writeValidNonNullableToJSON(mem memory.Allocator) error {
	arr := buildStructArray(mem, structRequired, []*int8{new(int8(1)), new(int8(2))})
	defer arr.Release()
	rec := array.NewRecordBatch(schemaRequired, []arrow.Array{arr}, int64(arr.Len()))
	defer rec.Release()

	jsonBuf, err := rec.MarshalJSON()
	if err != nil {
		return err
	}
	fmt.Printf("json: \n%s", jsonBuf)
	return nil
}
