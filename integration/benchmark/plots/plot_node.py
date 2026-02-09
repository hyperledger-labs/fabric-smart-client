import re
import sys
import os
import numpy as np
import matplotlib.pyplot as plt
from matplotlib.backends.backend_pdf import PdfPages
from collections import defaultdict

# ----------------------------
#  WORKLOAD STYLING
# ----------------------------
# Consistent colors and markers for workloads across all plots
# Colors are colorblind-friendly, markers provide additional distinction

WORKLOAD_STYLES = {
    'noop': {'color': '#0173B2', 'marker': 'o', 'label': 'w=noop'},      # Blue, circle
    'cpu': {'color': '#DE8F05', 'marker': 's', 'label': 'w=cpu'},        # Orange, square
    'ecdsa': {'color': '#029E73', 'marker': '^', 'label': 'w=ecdsa'},    # Green, triangle up
    'echo': {'color': '#CC78BC', 'marker': 'D', 'label': 'w=echo'},      # Purple, diamond
}

def get_workload_style(workload):
    """Get consistent style for a workload, with fallback for unknown workloads."""
    if workload in WORKLOAD_STYLES:
        return WORKLOAD_STYLES[workload]
    # Fallback for unknown workloads
    return {'color': '#949494', 'marker': 'v', 'label': f'w={workload}'}

# Connection count styles (different from workload styles)
NC_COLORS = ['#0173B2', '#DE8F05', '#029E73', '#CC78BC', '#ECE133', '#56B4E9']
NC_MARKERS = ['o', 's', '^', 'D', 'v', 'p']

def get_nc_style(nc, idx):
    """Get style for connection count."""
    return {
        'color': NC_COLORS[idx % len(NC_COLORS)],
        'marker': NC_MARKERS[idx % len(NC_MARKERS)],
        'label': f'nc={nc}'
    }

if len(sys.argv) < 2:
    print("Usage: python plot_node.py <benchmark_log> [output.pdf]")
    print("  If output.pdf is not provided, it will be generated from the input filename")
    sys.exit(1)

LOG_FILE = sys.argv[1]

# Generate output filename if not provided
if len(sys.argv) >= 3:
    OUT_PDF = sys.argv[2]
else:
    # Strip extension and replace with .pdf
    base_name = os.path.splitext(LOG_FILE)[0]
    OUT_PDF = f"{base_name}.pdf"

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
# Generate plots
# ------------------------------------------------

with PdfPages(OUT_PDF) as pdf:

    # ---- BenchmarkAPI ----
    # One plot per factory value, showing all workloads
    for factory in sorted(bench_api.keys()):
        plt.figure(figsize=(10, 6))
        
        print(f"\nBenchmarkAPI (f={factory})")
        
        for workload in sorted(bench_api[factory].keys()):
            worker_dict = bench_api[factory][workload]
            
            worker_counts = sorted(worker_dict.keys())
            tps_means = [np.mean(worker_dict[w]) for w in worker_counts]
            tps_stddev = [np.std(worker_dict[w]) for w in worker_counts]
            
            print(f"  workload={workload}")
            print("    workers:", worker_counts)
            print("    TPS:", tps_means)
            
            style = get_workload_style(workload)
            plt.errorbar(
                worker_counts,
                tps_means,
                yerr=tps_stddev,
                marker=style['marker'],
                color=style['color'],
                label=style['label'],
                capsize=4,
                linewidth=2,
                markersize=8,
            )
        
        plt.xlabel("Workers", fontsize=12)
        plt.ylabel("TPS", fontsize=12)
        plt.title(f"BenchmarkAPI (f={factory})", fontsize=14)
        plt.grid(True, alpha=0.3)
        plt.legend(title="Workload", fontsize=10)
        plt.tight_layout()
        
        pdf.savefig()
        plt.close()

    # ---- BenchmarkAPIGRPC ----
    # One plot per workload, showing all connection counts
    for workload in sorted(bench_api_grpc.keys()):
        plt.figure(figsize=(10, 6))
        
        print(f"\nBenchmarkAPIGRPC (workload={workload})")
        
        nc_list = sorted(bench_api_grpc[workload].keys())
        
        for idx, nc in enumerate(nc_list):
            worker_dict = bench_api_grpc[workload][nc]
            
            worker_counts = sorted(worker_dict.keys())
            tps_means = [np.mean(worker_dict[w]) for w in worker_counts]
            tps_stddev = [np.std(worker_dict[w]) for w in worker_counts]
            
            print(f"  nc={nc}")
            print("    workers:", worker_counts)
            print("    TPS:", tps_means)
            
            style = get_nc_style(nc, idx)
            plt.errorbar(
                worker_counts,
                tps_means,
                yerr=tps_stddev,
                marker=style['marker'],
                color=style['color'],
                label=style['label'],
                capsize=4,
                linewidth=2,
                markersize=8,
            )
        
        plt.xlabel("Workers", fontsize=12)
        plt.ylabel("TPS", fontsize=12)
        plt.title(f"BenchmarkAPIGRPC (w={workload})", fontsize=14)
        plt.grid(True, alpha=0.3)
        plt.legend(title="Connections", fontsize=10)
        plt.tight_layout()
        
        pdf.savefig()
        plt.close()

    # ---- BenchmarkAPIGRPCRemote ----
    # One plot per workload, showing all connection counts
    for workload in sorted(bench_api_grpc_remote.keys()):
        plt.figure(figsize=(10, 6))
        
        print(f"\nBenchmarkAPIGRPCRemote (workload={workload})")
        
        nc_list = sorted(bench_api_grpc_remote[workload].keys())
        
        for idx, nc in enumerate(nc_list):
            worker_dict = bench_api_grpc_remote[workload][nc]
            
            worker_counts = sorted(worker_dict.keys())
            tps_means = [np.mean(worker_dict[w]) for w in worker_counts]
            tps_stddev = [np.std(worker_dict[w]) for w in worker_counts]
            
            print(f"  nc={nc}")
            print("    workers:", worker_counts)
            print("    TPS:", tps_means)
            
            style = get_nc_style(nc, idx)
            plt.errorbar(
                worker_counts,
                tps_means,
                yerr=tps_stddev,
                marker=style['marker'],
                color=style['color'],
                label=style['label'],
                capsize=4,
                linewidth=2,
                markersize=8,
            )
        
        plt.xlabel("Workers", fontsize=12)
        plt.ylabel("TPS", fontsize=12)
        plt.title(f"BenchmarkAPIGRPCRemote (w={workload})", fontsize=14)
        plt.grid(True, alpha=0.3)
        plt.legend(title="Connections", fontsize=10)
        plt.tight_layout()
        
        pdf.savefig()
        plt.close()

print(f"\nSaved plots to: {OUT_PDF}")