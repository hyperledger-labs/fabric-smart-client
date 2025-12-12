import re
import sys
import numpy as np
import matplotlib.pyplot as plt
from matplotlib.backends.backend_pdf import PdfPages
from collections import defaultdict

# ----------------------------
#  HANDLE CLI ARGUMENTS
# ----------------------------

if len(sys.argv) < 3:
    print("Usage: python plot.py <bench_file> <output_pdf>")
    sys.exit(1)

INPUT_FILE = sys.argv[1]
OUTPUT_PDF = sys.argv[2]

# ----------------------------
#  PARSING LOGIC
# ----------------------------
#
# We need to extract:
#   - benchmark group name (e.g. "BenchmarkSimple/parallel")
#   - worker count (default = 1)
#   - TPS value
#
# ----------------------------

# dictionary:
# group_name → { worker → [tps runs] }
groups = defaultdict(lambda: defaultdict(list))

# Matches all forms:
#   BenchmarkSimple/parallel-4    ... 140240 TPS
#   BenchmarkParallelWork-12      ... 13246 TPS
pattern = re.compile(r"(Benchmark[^\s/-]+(?:/[^\s/-]+)?)"      # group name
                     r"(?:-(\d+))?"                           # optional worker/cpu suffix
                     r".*?([\d.]+)\s+TPS", re.IGNORECASE)

with open(INPUT_FILE, "r") as f:
    for line in f:
        m = pattern.search(line)
        if not m:
            continue

        group = m.group(1)
        worker_str = m.group(2)
        tps = float(m.group(3))

        worker = int(worker_str) if worker_str else 1

        groups[group][worker].append(tps)

with PdfPages(OUTPUT_PDF) as pdf:

    for group_name, worker_dict in sorted(groups.items()):

        # Sort and compute stats
        worker_counts = sorted(worker_dict.keys())
        tps_means = [np.mean(worker_dict[c]) for c in worker_counts]
        tps_stddev = [np.std(worker_dict[c]) for c in worker_counts]

        print(f"\nGroup: {group_name}")
        print(" Worker counts:", worker_counts)
        print(" TPS means:", tps_means)

        # -------------
        # Plot 1: TPS
        # -------------

        plt.figure(figsize=(10, 6))
        plt.errorbar(worker_counts, tps_means, yerr=tps_stddev, fmt="o-", capsize=6)
        plt.xlabel("Worker Count")
        plt.ylabel("Average TPS")
        plt.title(f"{group_name}")
        plt.grid(True)
        plt.tight_layout()
        pdf.savefig()
        plt.close()

print(f"\nSaved multi-benchmark PDF to: {OUTPUT_PDF}")