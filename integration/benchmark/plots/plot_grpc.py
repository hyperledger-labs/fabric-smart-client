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
#  EXTRACT GOGC FROM FILENAME
# ----------------------------

gc_match = re.search(r"benchmark_gc_([^./]+)", INPUT_FILE)
GOGC_LABEL = f"GOGC={gc_match.group(1)}" if gc_match else "GOGC=unknown"

# ----------------------------
#  PARSING LOGIC
# ----------------------------
#
# workload → connections → workers → [tps runs]
#
# ----------------------------

data = defaultdict(lambda: defaultdict(lambda: defaultdict(list)))

pattern = re.compile(
    r"benchmark/w=(\w+)/nc=(\d+)(?:-(\d+))?.*?([\d.]+)\s+TPS"
)

with open(INPUT_FILE, "r") as f:
    for line in f:
        m = pattern.search(line)
        if not m:
            continue

        workload = m.group(1)
        nc = int(m.group(2))
        worker = int(m.group(3)) if m.group(3) else 1
        tps = float(m.group(4))

        data[workload][nc][worker].append(tps)

# ----------------------------
#  PLOTTING
# ----------------------------

with PdfPages(OUTPUT_PDF) as pdf:

    for workload, conn_dict in sorted(data.items()):

        plt.figure(figsize=(10, 6))

        print(f"\nWorkload: {workload}")

        for nc in sorted(conn_dict.keys()):
            worker_dict = conn_dict[nc]

            worker_counts = sorted(worker_dict.keys())
            tps_means = [np.mean(worker_dict[w]) for w in worker_counts]
            tps_stddev = [np.std(worker_dict[w]) for w in worker_counts]

            print(f"  nc={nc}")
            print("    workers:", worker_counts)
            print("    TPS:", tps_means)

            plt.errorbar(
                worker_counts,
                tps_means,
                yerr=tps_stddev,
                marker="o",
                label=f"nc={nc}",
                capsize=4,
            )

        plt.xlabel("Workers")
        plt.ylabel("TPS")
        plt.title(f"Workload: {workload}")
        plt.suptitle(GOGC_LABEL, fontsize=12, fontweight="bold", y=0.98)

        plt.grid(True)
        plt.legend(title="Connections")
        plt.tight_layout(rect=[0, 0, 1, 0.95])

        pdf.savefig()
        plt.close()

print(f"\nSaved plots to: {OUTPUT_PDF}")
