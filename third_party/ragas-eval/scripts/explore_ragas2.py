#!/usr/bin/env python3
"""Deep explore ragas ToolCallAccuracy API."""

# Deprecated path but works for exploration
from ragas.metrics import ToolCallAccuracy
from ragas import SingleTurnSample
import inspect

tca = ToolCallAccuracy()
print("=== ToolCallAccuracy ===")
print("class:", ToolCallAccuracy)
print("strict_order:", tca.strict_order)
print("arg_comparison_metric:", tca.arg_comparison_metric)
print()

# Inspect single_turn_score
print("=== single_turn_score ===")
sig = inspect.signature(tca.single_turn_score)
print("signature:", sig)
src = inspect.getsource(tca.__class__.single_turn_score)
print("source:\n", src[:1500])

print("\n=== multi_turn_score ===")
sig = inspect.signature(tca.multi_turn_score)
print("signature:", sig)

# Explore required columns
print("\n=== required_columns ===")
print(tca.required_columns)

# Try SingleTurnSample for tool calls
print("\n=== SingleTurnSample for tool calls ===")
import json
sample = SingleTurnSample(
    reference_tool_calls=[
        {"name": "read_file", "arguments": json.dumps({"path": "README.md"})}
    ],
    actual_tool_calls=[
        {"name": "read_file", "arguments": json.dumps({"path": "README.md"})}
    ],
)
print("sample:", sample)
try:
    score = tca.single_turn_score(sample)
    print("score:", score)
except Exception as e:
    print("ERROR:", e)

# Check if there's a way to use it with tool_call data format
print("\n=== Source of _tool_call_accuracy ===")
import ragas.metrics._tool_call_accuracy as tca_mod
src = inspect.getsource(tca_mod)
print(src[:2000])
