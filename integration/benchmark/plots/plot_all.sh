#!/usr/bin/env bash
set -euo pipefail

PLOT_SCRIPT="plot.py"

for txt in benchmark_gc_*.txt; do
  # Strip .txt extension
  base="${txt%.txt}"
  pdf="${base}.pdf"

  echo "Generating ${pdf} from ${txt}"

  python3 "${PLOT_SCRIPT}" "${txt}" "${pdf}"
done

echo "All plots generated ✔"
