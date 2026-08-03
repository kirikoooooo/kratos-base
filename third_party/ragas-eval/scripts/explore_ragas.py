#!/usr/bin/env python3
"""Explore ragas tool-calling metrics."""
import ragas
import ragas.metrics as m

print("ragas version:", ragas.__version__)

# List all metrics
print("\n=== All ragas.metrics attributes ===")
for a in sorted(dir(m)):
    if a[0].isupper():
        print(f"  {a}")

# Check ToolCallAccuracy
print("\n=== ToolCallAccuracy ===")
try:
    from ragas.metrics import ToolCallAccuracy
    print("ToolCallAccuracy available")
    print("  class:", ToolCallAccuracy)
    tca = ToolCallAccuracy()
    print("  instance attrs:", [a for a in dir(tca) if not a.startswith('_')])
except ImportError as e:
    print(f"ToolCallAccuracy not available: {e}")

# Try alternate import paths
for path in [
    "ragas.metrics.ToolCallAccuracy",
    "ragas.metrics.agent.ToolCallAccuracy",
    "ragas.metrics._tool_call_accuracy.ToolCallAccuracy",
]:
    parts = path.rsplit(".", 1)
    try:
        mod = __import__(parts[0], fromlist=[parts[1]])
        cls = getattr(mod, parts[1])
        print(f"\nFOUND: {path} -> {cls}")
    except (ImportError, AttributeError) as e:
        print(f"  NOT FOUND: {path}: {e}")

# Check SingleTurnSample
print("\n=== SingleTurnSample ===")
try:
    from ragas import SingleTurnSample
    print("SingleTurnSample available")
    import inspect
    sig = inspect.signature(SingleTurnSample.__init__)
    print("  params:", list(sig.parameters.keys()))
except ImportError as e:
    print(f"SingleTurnSample not available: {e}")

# Check ragas.metrics submodules
print("\n=== ragas.metrics submodules ===")
import os, ragas
metrics_dir = os.path.dirname(m.__file__)
for f in sorted(os.listdir(metrics_dir)):
    if f.endswith('.py') and not f.startswith('_'):
        print(f"  {f}")
