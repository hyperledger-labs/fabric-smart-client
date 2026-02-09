import re
import sys
import numpy as np
import matplotlib.pyplot as plt
from matplotlib.backends.backend_pdf import PdfPages
from collections import defaultdict

if len(sys.argv) < 3:
    print("Usage: python plot_node.py <benchmark_log> <output.pdf>")
    sys.exit(1)

LOG_FILE = sys.argv[1]
OUT_PDF = sys.argv[2]

# ------------------------------------------------
# Data structures
# ------------------------------------------------

# BenchmarkAPI
# factory → workload → workers → [tps]
bench_api = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))

# BenchmarkAPIGRPC
# workload → nc → workers → [tps]
bench_api_grpc = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))

# BenchmarkAPIGRPCRemote
# workload → nc → workers → [tps]
bench_api_grpc_remote = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))

# ------------------------------------------------
# Regex patterns
# ------------------------------------------------

# Updated pattern for BenchmarkAPI/w=noop/f=0/nc=0-16
re_api = re.compile(
    r"BenchmarkAPI/"
    r"w=(\w+)/"
    r"f=(\d+)/"
    r"nc=\d+(?:-(\d+))?"
    r".*?([\d.]+)\s+TPS"
)

# Updated pattern for BenchmarkAPIGRPC/w=noop/f=1/nc=2-16
re_api_grpc = re.compile(
    r"BenchmarkAPIGRPC/"
    r"w=(\w+)/"
    r"f=\d+/"
    r"nc=(\d+)(?:-(\d+))?"
    r".*?([\d.]+)\s+TPS"
)

# Pattern for BenchmarkAPIGRPCRemote/w=noop/nc=2-16
re_api_grpc_remote = re.compile(
    r"BenchmarkAPIGRPCRemote/"
    r"w=(\w+)/"
    r"nc=(\d+)(?:-(\d+))?"
    r".*?([\d.]+)\s+TPS"
)

# ------------------------------------------------
# Parse file
# ------------------------------------------------

with open(LOG_FILE) as f:
    for line in f:
        if m := re_api.search(line):
            workload, factory, workers, tps = m.groups()
            workers = int(workers) if workers else 1
            bench_api[int(factory)][workload][workers].append(float(tps))
            continue

        if m := re_api_grpc.search(line):
            workload, nc, workers, tps = m.groups()
            workers = int(workers) if workers else 1
            bench_api_grpc[workload][int(nc)][workers].append(float(tps))
            continue

        if m := re_api_grpc_remote.search(line):
            workload, nc, workers, tps = m.groups()
            workers = int(workers) if workers else 1
            bench_api_grpc_remote[workload][int(nc)][workers].append(float(tps))
            continue

# ------------------------------------------------
# Helper plot function
# ------------------------------------------------

def plot_grouped(data, title, xlabel, ylabel, legend_title):
    plt.figure(figsize=(10, 6))

    for label in sorted(data.keys()):
        xs = sorted(data[label].keys())
        ys = [np.mean(data[label][x]) for x in xs]
        yerr = [np.std(data[label][x]) for x in xs]

        plt.errorbar(xs, ys, yerr=yerr, marker="o", capsize=4, label=label)

    plt.xlabel(xlabel)
    plt.ylabel(ylabel)
    plt.title(title)
    plt.grid(True)
    plt.legend(title=legend_title)
    plt.tight_layout()

# ------------------------------------------------
# Generate plots
# ------------------------------------------------

with PdfPages(OUT_PDF) as pdf:

    # ---- BenchmarkAPI ----
    for factory in sorted(bench_api.keys()):
        plot_grouped(
            bench_api[factory],
            f"BenchmarkAPI (f={factory})",
            "Workers",
            "TPS",
            "Workload",
        )
        pdf.savefig()
        plt.close()

    # ---- BenchmarkAPIGRPC ----
    for workload in sorted(bench_api_grpc.keys()):
        plot_grouped(
            bench_api_grpc[workload],
            f"BenchmarkAPIGRPC ({workload})",
            "Workers",
            "TPS",
            "Connections (nc)",
        )
        pdf.savefig()
        plt.close()

    # ---- BenchmarkAPIGRPCRemote ----
    for workload in sorted(bench_api_grpc_remote.keys()):
        plot_grouped(
            bench_api_grpc_remote[workload],
            f"BenchmarkAPIGRPCRemote ({workload})",
            "Workers",
            "TPS",
            "Connections (nc)",
        )
        pdf.savefig()
        plt.close()

print(f"Plots written to {OUT_PDF}")