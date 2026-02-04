import re
import sys
import numpy as np
import matplotlib.pyplot as plt
from matplotlib.backends.backend_pdf import PdfPages
from collections import defaultdict

# ----------------------------
# CLI
# ----------------------------

if len(sys.argv) < 3:
    print("Usage: python plot_ecdsasign.py <benchmark_log> <output.pdf>")
    sys.exit(1)

LOG_FILE = sys.argv[1]
OUT_PDF = sys.argv[2]

# ----------------------------
# Data structure
# benchmark → workers → [tps]
# ----------------------------

data = defaultdict(lambda: defaultdict(list))

# ----------------------------
# Regex
# ----------------------------
#
# Examples:
# BenchmarkECDSASign-64   ...   412400 TPS
# BenchmarkECDSASign2        ...   25545 TPS
#
# ----------------------------

pattern = re.compile(
    r"(Benchmark\w+?)(?:-(\d+))?\s+.*?([\d.]+)\s+TPS"
)

# ----------------------------
# Parse file
# ----------------------------

with open(LOG_FILE) as f:
    for line in f:
        m = pattern.search(line)
        if not m:
            continue

        bench, workers, tps = m.groups()
        workers = int(workers) if workers else 1
        data[bench][workers].append(float(tps))

# ----------------------------
# Plot
# ----------------------------

with PdfPages(OUT_PDF) as pdf:
    plt.figure(figsize=(10, 6))

    for bench in sorted(data.keys()):
        worker_counts = sorted(data[bench].keys())
        tps_means = [np.mean(data[bench][w]) for w in worker_counts]
        tps_stddev = [np.std(data[bench][w]) for w in worker_counts]

        print(f"\n{bench}")
        print("  workers:", worker_counts)
        print("  TPS:", tps_means)

        plt.errorbar(
            worker_counts,
            tps_means,
            yerr=tps_stddev,
            marker="o",
            capsize=4,
            label=bench,
        )

    plt.xlabel("Workers")
    plt.ylabel("TPS")
    plt.title("View Benchmark Throughput")
    plt.grid(True)
    plt.legend(title="Benchmark")
    plt.tight_layout()

    pdf.savefig()
    plt.close()

print(f"\nSaved plot to {OUT_PDF}")
