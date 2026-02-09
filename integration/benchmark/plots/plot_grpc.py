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

# ----------------------------
#  HANDLE CLI ARGUMENTS
# ----------------------------

if len(sys.argv) < 2:
    print("Usage: python plot_grpc.py <bench_file> [output_pdf]")
    print("  If output_pdf is not provided, it will be generated from the input filename")
    sys.exit(1)

INPUT_FILE = sys.argv[1]

# Generate output filename if not provided
if len(sys.argv) >= 3:
    OUTPUT_PDF = sys.argv[2]
else:
    # Strip extension and replace with .pdf
    base_name = os.path.splitext(INPUT_FILE)[0]
    OUTPUT_PDF = f"{base_name}.pdf"

# ----------------------------
#  EXTRACT GOGC FROM FILENAME
# ----------------------------

gc_match = re.search(r"benchmark_gc_([^./]+)", INPUT_FILE)
GOGC_LABEL = f"GOGC={gc_match.group(1)}" if gc_match else ""

# ----------------------------
#  PARSING LOGIC
# ----------------------------
#
# Data structure: benchmark_type → workload → signer/nc → workers → [tps runs]
#
# Supports three formats:
# 1. BenchmarkGRPCBaseline/w=noop-16
# 2. BenchmarkGRPCImpl/w=noop/grpcsigner=mock-16
# 3. benchmarkRemote/w=echo/nc=1-8
#
# ----------------------------

data = defaultdict(lambda: defaultdict(lambda: defaultdict(lambda: defaultdict(list))))

# Pattern for BenchmarkGRPCBaseline/w=noop-16
pattern_baseline = re.compile(
    r"BenchmarkGRPCBaseline/w=(\w+)(?:-(\d+))?.*?([\d.]+)\s+TPS"
)

# Pattern for BenchmarkGRPCImpl/w=noop/grpcsigner=mock-16
pattern_impl = re.compile(
    r"BenchmarkGRPCImpl/w=(\w+)/grpcsigner=(\w+)(?:-(\d+))?.*?([\d.]+)\s+TPS"
)

# Pattern for benchmarkRemote/w=echo/nc=1-8
pattern_remote = re.compile(
    r"benchmarkRemote/w=(\w+)/nc=(\d+)(?:-(\d+))?.*?([\d.]+)\s+TPS"
)

with open(INPUT_FILE, "r") as f:
    for line in f:
        # Try baseline pattern first
        m = pattern_baseline.search(line)
        if m:
            workload = m.group(1)
            worker = int(m.group(2)) if m.group(2) else 1
            tps = float(m.group(3))
            
            data["Baseline"][workload]["baseline"][worker].append(tps)
            continue
        
        # Try impl pattern
        m = pattern_impl.search(line)
        if m:
            workload = m.group(1)
            signer = m.group(2)
            worker = int(m.group(3)) if m.group(3) else 1
            tps = float(m.group(4))
            
            data["Impl"][workload][signer][worker].append(tps)
            continue
        
        # Try remote pattern
        m = pattern_remote.search(line)
        if m:
            workload = m.group(1)
            nc = int(m.group(2))
            worker = int(m.group(3)) if m.group(3) else 1
            tps = float(m.group(4))
            
            data["Remote"][workload][f"nc={nc}"][worker].append(tps)
            continue

# ----------------------------
#  PLOTTING
# ----------------------------

with PdfPages(OUTPUT_PDF) as pdf:
    
    # Plot 1: BenchmarkGRPCBaseline - all workloads on one plot
    if "Baseline" in data:
        plt.figure(figsize=(10, 6))
        
        print("\nBenchmarkGRPCBaseline")
        
        for workload in sorted(data["Baseline"].keys()):
            worker_dict = data["Baseline"][workload]["baseline"]
            
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
        plt.title("BenchmarkGRPCBaseline", fontsize=14)
        if GOGC_LABEL:
            plt.suptitle(GOGC_LABEL, fontsize=12, fontweight="bold", y=0.98)
        
        plt.grid(True, alpha=0.3)
        plt.legend(title="Workload", fontsize=10)
        plt.tight_layout(rect=[0, 0, 1, 0.95] if GOGC_LABEL else [0, 0, 1, 1])
        
        pdf.savefig()
        plt.close()
    
    # Plot 2+: BenchmarkGRPCImpl - one plot per signer, all workloads on each plot
    if "Impl" in data:
        # Reorganize data: signer → workload → workers → [tps]
        by_signer = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))
        
        for workload in data["Impl"]:
            for signer in data["Impl"][workload]:
                for worker in data["Impl"][workload][signer]:
                    by_signer[signer][workload][worker] = data["Impl"][workload][signer][worker]
        
        # Create one plot per signer
        for signer in sorted(by_signer.keys()):
            plt.figure(figsize=(10, 6))
            
            print(f"\nBenchmarkGRPCImpl (grpcsigner={signer})")
            
            for workload in sorted(by_signer[signer].keys()):
                worker_dict = by_signer[signer][workload]
                
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
            plt.title(f"BenchmarkGRPCImpl (grpcsigner={signer})", fontsize=14)
            if GOGC_LABEL:
                plt.suptitle(GOGC_LABEL, fontsize=12, fontweight="bold", y=0.98)
            
            plt.grid(True, alpha=0.3)
            plt.legend(title="Workload", fontsize=10)
            plt.tight_layout(rect=[0, 0, 1, 0.95] if GOGC_LABEL else [0, 0, 1, 1])
            
            pdf.savefig()
            plt.close()
    
    # Plot 3+: BenchmarkRemote - one plot per workload, all connection counts on each plot
    if "Remote" in data:
        # Connection count styles (different from workload styles)
        nc_colors = ['#0173B2', '#DE8F05', '#029E73', '#CC78BC', '#ECE133', '#56B4E9']
        nc_markers = ['o', 's', '^', 'D', 'v', 'p']
        
        for workload in sorted(data["Remote"].keys()):
            plt.figure(figsize=(10, 6))
            
            print(f"\nBenchmarkRemote (workload={workload})")
            
            nc_list = sorted(data["Remote"][workload].keys(), key=lambda x: int(x.split('=')[1]))
            
            for idx, nc in enumerate(nc_list):
                worker_dict = data["Remote"][workload][nc]
                
                worker_counts = sorted(worker_dict.keys())
                tps_means = [np.mean(worker_dict[w]) for w in worker_counts]
                tps_stddev = [np.std(worker_dict[w]) for w in worker_counts]
                
                print(f"  {nc}")
                print("    workers:", worker_counts)
                print("    TPS:", tps_means)
                
                color = nc_colors[idx % len(nc_colors)]
                marker = nc_markers[idx % len(nc_markers)]
                
                plt.errorbar(
                    worker_counts,
                    tps_means,
                    yerr=tps_stddev,
                    marker=marker,
                    color=color,
                    label=nc,
                    capsize=4,
                    linewidth=2,
                    markersize=8,
                )
            
            plt.xlabel("Workers", fontsize=12)
            plt.ylabel("TPS", fontsize=12)
            plt.title(f"BenchmarkRemote (w={workload})", fontsize=14)
            if GOGC_LABEL:
                plt.suptitle(GOGC_LABEL, fontsize=12, fontweight="bold", y=0.98)
            
            plt.grid(True, alpha=0.3)
            plt.legend(title="Connections", fontsize=10)
            plt.tight_layout(rect=[0, 0, 1, 0.95] if GOGC_LABEL else [0, 0, 1, 1])
            
            pdf.savefig()
            plt.close()

print(f"\nSaved plots to: {OUTPUT_PDF}")