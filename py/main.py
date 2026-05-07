from collections.abc import Callable
from io import BytesIO
import json
import pyarrow as pa
from pyarrow import json as pa_json


schema_required = pa.schema(
    fields=[
        pa.field(
            name="parent",
            type=pa.struct(
                fields=[
                    pa.field(
                        name="child",
                        type=pa.int8(),
                        nullable=False,
                    )
                ]
            ),
            nullable=True,
        )
    ]
)

schema_nullable = pa.schema(
    fields=[
        pa.field(
            name="parent",
            type=pa.struct(
                fields=[
                    pa.field(
                        name="child",
                        type=pa.int8(),
                        nullable=True,
                    )
                ]
            ),
            nullable=True,
        )
    ]
)


def try_run(name: str, fn: Callable[[], None]) -> None:
    print(f"** {name} **")
    try:
        fn()
    except Exception as e:
        print(f"Error: {e}")
    print("---------------")


def main() -> None:
    try_run(
        "load json with nulls in required field", load_json_with_nulls_in_required_field
    )
    try_run("create table from invalid arrays", create_table_from_invalid_arrays)
    try_run("cast nullable into non-nullable", cast_to_non_nullable)
    try_run(
        "write table with nulls into non-nullable IPC stream",
        write_nullable_table_into_non_nullable_ipc,
    )
    try_run("write valid non-nullable array to JSON", write_valid_non_nullable_to_json)
    # NOTE: there is no way to write to IPC then read it with a custom schema. You'll always get
    # record batches in the schema you wrote
    # See: https://arrow.apache.org/docs/python/generated/pyarrow.ipc.open_stream.html#pyarrow.ipc.open_stream


def load_json_with_nulls_in_required_field() -> None:
    original_payload = b'{"parent": {"child": 1}}\n{"parent": {"child": null}}'
    table = pa_json.read_json(
        input_file=BytesIO(original_payload),
        parse_options=pa_json.ParseOptions(
            explicit_schema=schema_required,
        ),
    )
    print(table)


def create_table_from_invalid_arrays() -> None:
    table = pa.table(
        [pa.array([{"child": 1}, {"child": None}])], schema=schema_required
    )
    print(table)


def cast_to_non_nullable() -> None:
    nullable_table = pa.table(
        [pa.array([{"child": 1}, {"child": None}])], schema=schema_nullable
    )
    table = nullable_table.cast(schema_required)
    print(table)


def write_nullable_table_into_non_nullable_ipc() -> None:
    nullable_table = pa.table(
        [pa.array([{"child": 1}, {"child": None}])], schema=schema_nullable
    )
    sink = pa.BufferOutputStream()
    with pa.ipc.new_file(sink, schema_required) as writer:
        writer.write_table(nullable_table)
    buf = sink.getvalue()
    print(buf)


def write_valid_non_nullable_to_json() -> None:
    table = pa.table([pa.array([{"child": 1}, {"child": 2}])], schema=schema_required)
    # NOTE: there is no explicit to_json() method. These are all the ways I
    # I could find of dumping a pa.Table to JSON
    pylist_json = json.dumps(table.to_pylist())
    pandas_json = table.to_pandas().to_json(orient="values")
    print("pylist json:", pylist_json)
    print("pandas json:", pandas_json)


if __name__ == "__main__":
    main()
