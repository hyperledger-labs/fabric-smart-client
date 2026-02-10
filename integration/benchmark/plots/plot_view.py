import re
import sys
import os
import numpy as np
import matplotlib.pyplot as plt
from matplotlib.backends.backend_pdf import PdfPages
from collections import defaultdict

# ----------------------------
#  BENCHMARK STYLING
# ----------------------------
# Consistent colors and markers for benchmarks
# Colors are colorblind-friendly, markers provide additional distinction

BENCHMARK_STYLES = {
    'BenchmarkECDSASign': {'color': '#0173B2', 'marker': 'o'},      # Blue, circle
    'BenchmarkECDSASign2': {'color': '#DE8F05', 'marker': 's'},     # Orange, square
    'BenchmarkECDSAVerify': {'color': '#029E73', 'marker': '^'},    # Green, triangle up
    'BenchmarkECDSAVerify2': {'color': '#CC78BC', 'marker': 'D'},   # Purple, diamond
}

# Fallback colors and markers for unknown benchmarks
FALLBACK_COLORS = ['#ECE133', '#56B4E9', '#949494', '#F0E442']
FALLBACK_MARKERS = ['v', 'p', '<', '>']

def get_benchmark_style(benchmark, idx):
    """Get consistent style for a benchmark, with fallback for unknown benchmarks."""
    if benchmark in BENCHMARK_STYLES:
        return BENCHMARK_STYLES[benchmark]
    # Fallback for unknown benchmarks
    return {
        'color': FALLBACK_COLORS[idx % len(FALLBACK_COLORS)],
        'marker': FALLBACK_MARKERS[idx % len(FALLBACK_MARKERS)]
    }

# ----------------------------
# CLI
# ----------------------------

if len(sys.argv) < 2:
    print("Usage: python plot_view.py <benchmark_log> [output.pdf]")
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

    for idx, bench in enumerate(sorted(data.keys())):
        worker_counts = sorted(data[bench].keys())
        tps_means = [np.mean(data[bench][w]) for w in worker_counts]
        tps_stddev = [np.std(data[bench][w]) for w in worker_counts]

        print(f"\n{bench}")
        print("  workers:", worker_counts)
        print("  TPS:", tps_means)

        style = get_benchmark_style(bench, idx)
        plt.errorbar(
            worker_counts,
            tps_means,
            yerr=tps_stddev,
            marker=style['marker'],
            color=style['color'],
            label=bench,
            capsize=4,
            linewidth=2,
            markersize=8,
        )

    plt.xlabel("Workers", fontsize=12)
    plt.ylabel("TPS", fontsize=12)
    plt.title("View Benchmark Throughput", fontsize=14)
    plt.grid(True, alpha=0.3)
    plt.legend(title="Benchmark", fontsize=10)
    plt.tight_layout()

    pdf.savefig()
    plt.close()

print(f"\nSaved plot to {OUT_PDF}")
