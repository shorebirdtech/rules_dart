"""What the Dart SDK's heavier actions use, for Bazel's local scheduler.

Bazel assumes an action takes one core and 250 MB. The SDK's tools run in one
Dart VM and use about one core, but the front end, the analysis server and
`gen_snapshot` hold a whole program in memory. Measured peaks, on rules_dart's
own targets (the `dart/ext` builder shims, which import the analyzer and
`package:build`): kernel 455–546 MB, analysis 549–640 MB, native compile
688–828 MB. Each declares its upper figure rounded up, so the scheduler stops
before the machine swaps rather than after.

The analyzer's peak grows with the program it loads, so its estimate is chosen
when the aspect runs, from the number of `.dart` files the analyzed program can
see: its own, its package's and every dependency's, the SDK aside. Bazel hands
`resource_set` only the action's input count, which counts a staged package
tree as one file, so it can't see that size itself. The callback must be a
top-level function, so the sizes are a few fixed tiers.

Cold peaks of `dart analyze`, analyzer and runner together, on a Flutter app's
targets (macOS; Linux CI runs about 1.25 times higher): 180–280 MB up to 1,800
files, 455 MB at 3,177, 921 MB at 3,543 (the first that imports Flutter) and
954 MB, near 1.2 GB on Linux, at 4,544. `dart fix` peaks within 10% of it on
the same target. The top tier is past what was measured; it keeps a larger
program from taking a smaller one's share.
"""

def _analysis_500(_os, _inputs):
    return {"cpu": 1, "memory": 500}

def _analysis_800(_os, _inputs):
    return {"cpu": 1, "memory": 800}

def _analysis_1600(_os, _inputs):
    return {"cpu": 1, "memory": 1600}

def _analysis_2400(_os, _inputs):
    return {"cpu": 1, "memory": 2400}

def analysis_resources(dart_files):
    """Picks the `resource_set` for analyzing a program.

    Args:
      dart_files: How many `.dart` files the analyzed program can see, the
        SDK's aside.

    Returns:
      A top-level `resource_set` callback.
    """
    if dart_files < 1000:
        return _analysis_500
    if dart_files < 2500:
        return _analysis_800
    if dart_files < 6000:
        return _analysis_1600
    return _analysis_2400

def kernel_resources(_os, _inputs):
    return {"cpu": 1, "memory": 600}

def native_compile_resources(_os, _inputs):
    return {"cpu": 1, "memory": 900}
