# Testing of internal field nullables across PyArrow and Arrow Go

## Problem statement

Given a schema like 
```
schema:
  fields:
    - name: "parent"
      nullable: true
      type: struct
      struct:
        fields:
        - name: "child"
          nullable: false
          type: int8
```

We'll call "inconsistent state" any state where there are nulls in the `child` buffer. 

In other words, this is a valid state
```
parent:
  valids: [1, 0]
  fields:
    - child:
        # no nulls in child, so it's valid
        valids: [1, 1]
        data: [1, 2]
```

whereas this is an invalid state
```
parent:
  valids: [1, 1]
  fields:
    - child:
        # null in child, invalid since schema says it's not nullable
        valids: [1, 0]
        data: [1, 2]
```

We want to know, for PyArrow and Arrow Go:
- Can you create invalid state using public APIs?
- Can you create invalid state by loading untrusted IPC/JSON data?
- What happens if you write the invalid state into IPC/JSON?

## Run it yourself

```
# PyArrow
cd py 
uv venv .venv
.venv/bin/activate
uv pip install -r requirements.txt
python main.py

# Arrow Go
cd go
go run main.go
```

## Findings

PyArrow:
1. I could not find a way to get PyArrow to create an invalid state using Public APIs
2. It throws an error when you load JSON which contains nulls in the child field and you're loading it with an expected schema
3. It throws an error when writing to IPC if the inner field of the Table is nullable but the IPC-stream expects non-nullable

Arrow Go:
1. Most APIs also raise an error, but I did manage to create invalid state using the `array.StructBuilder`. See `appendNullToInnerFieldUsingBuilder()`.
2. Writing JSON from an invalid state (created in [1]) results in the wrong JSON. See `appendNullToInnerFieldUsingBuilder()`
3. Loading JSON which contains nulls in the child field creates invalid state. See `loadJSONWithNullsInRequiredField`


## Conclusion and next steps

My previous approach was not in-line with C++/PyArrow:
- My changes were about making the JSON de/encoder "self-healing" so that even if there is invalid state, it would produce valid JSON. It looks like PyArrow's approach is to validate at every point possible to make it impossible to have invalid state in the first place
- My `array.Equals` change would add an optional way of ignoring nulls, effectively making it resilient to invalid state. Looks like C++ does not do that ([src](https://github.com/apache/arrow/blob/7339a2995daa3cd90f3c015a459a59fb65bc5c12/cpp/src/arrow/compare.cc#L223)).

The proper approach for Arrow Go:
- Make `StructBuilder.NewArray()` recursively validate that the `NullN()` of all child arrays is zero if the corresponding field is non-nullable. Otherwise it should panic.
- Maybe add `StructBuilder.UnsafeNewArray()` or some other similar method that means "I know what I'm doing, skip validation and just instance the thing. If it's wrong, it'll be Undefined Behavior".
- Make the `array.RecordFromJSON` and all other JSON-reading utilities validate it does not receive any `null` values if the field is non-nullable. Otherwise it should return an error, like PyArrow does.
- Go over all other forms of instantiating Arrays/RecordBatches/Tables and make sure that there is no way of generating invalid states. If there is, we should also add validation there and add some kind of `Unsafe*()` method that means you trust the data you're loading and you're OK with Undefined Behavior if that invariant is violated.

