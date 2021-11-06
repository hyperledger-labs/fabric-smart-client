package monitoring

const DashboardFabricBackendTemplate = `{
  "annotations": {
    "list": [
      {
        "builtIn": 1,
        "datasource": "-- Grafana --",
        "enable": true,
        "hide": true,
        "iconColor": "rgba(0, 211, 255, 1)",
        "name": "Annotations & Alerts",
        "type": "dashboard"
      }
    ]
  },
  "editable": true,
  "gnetId": 10716,
  "graphTooltip": 0,
  "id": 1,
  "links": [],
  "panels": [
    {
      "collapsed": false,
      "datasource": null,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 0
      },
      "id": 18,
      "panels": [],
      "title": "Hyperledger Fabric Metrics",
      "type": "row"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorPostfix": false,
      "colorValue": true,
      "colors": [
        "#d44a3a",
        "#5794F2",
        "#1F60C4"
      ],
      "datasource": null,
      "description": "The active version of Fabric.",
      "format": "none",
      "gauge": {
        "maxValue": 100,
        "minValue": 0,
        "show": false,
        "thresholdLabels": false,
        "thresholdMarkers": true
      },
      "gridPos": {
        "h": 4,
        "w": 4,
        "x": 0,
        "y": 1
      },
      "id": 2,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "options": {},
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": false,
        "lineColor": "rgb(31, 120, 193)",
        "show": false
      },
      "tableColumn": "version",
      "targets": [
        {
          "expr": "fabric_version",
          "format": "table",
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "",
      "timeFrom": null,
      "timeShift": null,
      "title": "Fabric Version",
      "type": "singlestat",
      "valueFontSize": "120%",
      "valueMaps": [
        {
          "op": "=",
          "text": "N/A",
          "value": "null"
        }
      ],
      "valueName": "first"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorValue": true,
      "colors": [
        "#FADE2A",
        "#FADE2A",
        "#d44a3a"
      ],
      "datasource": null,
      "description": "The active version of Go.",
      "format": "none",
      "gauge": {
        "maxValue": 100,
        "minValue": 0,
        "show": false,
        "thresholdLabels": false,
        "thresholdMarkers": true
      },
      "gridPos": {
        "h": 4,
        "w": 4,
        "x": 4,
        "y": 1
      },
      "id": 11,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "options": {},
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": false,
        "lineColor": "rgb(31, 120, 193)",
        "show": false
      },
      "tableColumn": "version",
      "targets": [
        {
          "expr": "go_info",
          "format": "table",
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "",
      "timeFrom": null,
      "timeShift": null,
      "title": "Go Version",
      "type": "singlestat",
      "valueFontSize": "80%",
      "valueMaps": [
        {
          "op": "=",
          "text": "N/A",
          "value": "null"
        }
      ],
      "valueName": "first"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorPostfix": false,
      "colorValue": true,
      "colors": [
        "#73BF69",
        "#FADE2A",
        "#d44a3a"
      ],
      "datasource": null,
      "description": "Height of the chain in blocks. ",
      "format": "none",
      "gauge": {
        "maxValue": 1000,
        "minValue": 0,
        "show": false,
        "thresholdLabels": false,
        "thresholdMarkers": false
      },
      "gridPos": {
        "h": 4,
        "w": 4,
        "x": 8,
        "y": 1
      },
      "id": 36,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "options": {},
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": false,
        "lineColor": "rgb(31, 120, 193)",
        "show": false
      },
      "tableColumn": "Value",
      "targets": [
        {
          "expr": "ledger_blockchain_height",
          "format": "table",
          "instant": true,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "500, 800, 1000",
      "timeFrom": null,
      "timeShift": null,
      "title": "Ledger Block Height",
      "type": "singlestat",
      "valueFontSize": "120%",
      "valueMaps": [
        {
          "op": "=",
          "text": "N/A",
          "value": "null"
        }
      ],
      "valueName": "max"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorPostfix": false,
      "colorValue": true,
      "colors": [
        "#d44a3a",
        "#FADE2A",
        "#73BF69"
      ],
      "datasource": null,
      "description": "Number of transactions processed.",
      "format": "none",
      "gauge": {
        "maxValue": 1000,
        "minValue": 0,
        "show": false,
        "thresholdLabels": false,
        "thresholdMarkers": false
      },
      "gridPos": {
        "h": 4,
        "w": 4,
        "x": 12,
        "y": 1
      },
      "id": 37,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "options": {},
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": false,
        "lineColor": "rgb(31, 120, 193)",
        "show": false
      },
      "tableColumn": "Value",
      "targets": [
        {
          "expr": "ledger_transaction_count",
          "format": "table",
          "instant": true,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "500, 800, 1000",
      "timeFrom": null,
      "timeShift": null,
      "title": "Ledger Transaction Count",
      "type": "singlestat",
      "valueFontSize": "120%",
      "valueMaps": [
        {
          "op": "=",
          "text": "N/A",
          "value": "null"
        }
      ],
      "valueName": "max"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorPostfix": false,
      "colorValue": true,
      "colors": [
        "#FADE2A",
        "#FADE2A",
        "#73BF69"
      ],
      "datasource": null,
      "description": "The number of proposals received.",
      "format": "none",
      "gauge": {
        "maxValue": 1000,
        "minValue": 0,
        "show": false,
        "thresholdLabels": false,
        "thresholdMarkers": false
      },
      "gridPos": {
        "h": 4,
        "w": 4,
        "x": 16,
        "y": 1
      },
      "id": 19,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "options": {},
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": false,
        "lineColor": "rgb(31, 120, 193)",
        "show": false
      },
      "tableColumn": "Value",
      "targets": [
        {
          "expr": "endorser_proposals_received",
          "format": "table",
          "instant": true,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "1000, 5000 10000",
      "timeFrom": null,
      "timeShift": null,
      "title": "Endorsment Proposal Requests",
      "type": "singlestat",
      "valueFontSize": "120%",
      "valueMaps": [
        {
          "op": "=",
          "text": "N/A",
          "value": "null"
        }
      ],
      "valueName": "max"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorPostfix": false,
      "colorValue": true,
      "colors": [
        "#F2495C",
        "#FADE2A",
        "#73BF69"
      ],
      "datasource": null,
      "description": "The number of successful proposals. ",
      "format": "none",
      "gauge": {
        "maxValue": 1000,
        "minValue": 0,
        "show": false,
        "thresholdLabels": false,
        "thresholdMarkers": false
      },
      "gridPos": {
        "h": 4,
        "w": 4,
        "x": 20,
        "y": 1
      },
      "id": 10,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "options": {},
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": false,
        "lineColor": "rgb(31, 120, 193)",
        "show": false
      },
      "tableColumn": "Value",
      "targets": [
        {
          "expr": "endorser_successful_proposals",
          "format": "table",
          "instant": true,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "1000, 5000, 10000",
      "timeFrom": null,
      "timeShift": null,
      "title": "Endorsment Proposal Successful",
      "type": "singlestat",
      "valueFontSize": "120%",
      "valueMaps": [
        {
          "op": "=",
          "text": "N/A",
          "value": "null"
        }
      ],
      "valueName": "max"
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "Time taken in seconds for the function to complete request to CouchDB.",
      "fill": 5,
      "fillGradient": 0,
      "gridPos": {
        "h": 4,
        "w": 24,
        "x": 0,
        "y": 5
      },
      "hiddenSeries": false,
      "id": 32,
      "legend": {
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 2,
      "links": [],
      "nullPointMode": "null",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [
        {
          "alias": "CouchDB"
        }
      ],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(couchdb_processing_time_bucket[5m])) by (le))",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "CouchDB",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Couch DB Processing Time",
      "tooltip": {
        "shared": true,
        "sort": 1,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "s",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "collapsed": false,
      "datasource": null,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 9
      },
      "id": 21,
      "panels": [],
      "title": "Go Metrics",
      "type": "row"
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "go_memstats_alloc_bytes",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 5,
        "w": 12,
        "x": 0,
        "y": 10
      },
      "hiddenSeries": false,
      "id": 23,
      "legend": {
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "null as zero",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "go_memstats_alloc_bytes",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{job}}",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Go Memory Allocations",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "go_memstats_alloc_bytes",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 5,
        "w": 12,
        "x": 12,
        "y": 10
      },
      "hiddenSeries": false,
      "id": 27,
      "legend": {
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "null as zero",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "rate(go_memstats_heap_objects{job!=\"prometheus\"}[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{job}}",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Go Heap Objects",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": true,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "go_gc_duration_seconds",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 6,
        "w": 12,
        "x": 0,
        "y": 15
      },
      "hiddenSeries": false,
      "id": 25,
      "legend": {
        "alignAsTable": true,
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "rightSide": true,
        "show": false,
        "total": false,
        "values": false
      },
      "lines": false,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "connected",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": true,
      "steppedLine": false,
      "targets": [
        {
          "expr": "rate(go_gc_duration_seconds[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{instance}}/{{job}}",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Go Garbage Collection",
      "tooltip": {
        "shared": false,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "go_gc_duration_seconds",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 6,
        "w": 12,
        "x": 12,
        "y": 15
      },
      "hiddenSeries": false,
      "id": 26,
      "legend": {
        "alignAsTable": true,
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "rightSide": true,
        "show": false,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "connected",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": true,
      "steppedLine": false,
      "targets": [
        {
          "expr": "rate(go_memstats_frees_total[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{instance}}/{{job}}",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Go Free Memory",
      "tooltip": {
        "shared": false,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "collapsed": false,
      "datasource": null,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 21
      },
      "id": 30,
      "panels": [],
      "title": "Ledger Metrics",
      "type": "row"
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "Time taken in seconds for ledger block processing.",
      "fill": 5,
      "fillGradient": 0,
      "gridPos": {
        "h": 4,
        "w": 12,
        "x": 0,
        "y": 22
      },
      "hiddenSeries": false,
      "id": 34,
      "legend": {
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 2,
      "links": [],
      "nullPointMode": "null",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(ledger_block_processing_time_bucket[5m])) by (le))",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": " ",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Ledger Block Processing Time",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "Time taken in seconds for committing block changes to state db.",
      "fill": 5,
      "fillGradient": 0,
      "gridPos": {
        "h": 4,
        "w": 12,
        "x": 12,
        "y": 22
      },
      "hiddenSeries": false,
      "id": 35,
      "legend": {
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 2,
      "links": [],
      "nullPointMode": "null",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(ledger_statedb_commit_time_bucket[5m])) by (le))",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": " ",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "StateDB Commit Time",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "collapsed": false,
      "datasource": null,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 26
      },
      "id": 4,
      "panels": [],
      "title": "Orderer Metrics",
      "type": "row"
    },
    {
      "cacheTimeout": null,
      "columns": [],
      "datasource": null,
      "description": "The number of transactions processed.",
      "fontSize": "100%",
      "gridPos": {
        "h": 6,
        "w": 24,
        "x": 0,
        "y": 27
      },
      "id": 8,
      "interval": null,
      "links": [],
      "maxDataPoints": 100,
      "options": {},
      "pageSize": null,
      "scroll": true,
      "showHeader": true,
      "sort": {
        "col": null,
        "desc": false
      },
      "styles": [
        {
          "alias": "Time",
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "link": false,
          "pattern": "Time",
          "type": "date"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "decimals": 2,
          "pattern": "/.*/",
          "thresholds": [],
          "type": "number",
          "unit": "short"
        }
      ],
      "targets": [
        {
          "expr": "broadcast_processed_count",
          "format": "table",
          "instant": false,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "timeFrom": null,
      "timeShift": null,
      "title": "Orderer Broadcasts",
      "transform": "table",
      "type": "table"
    },
    {
      "collapsed": false,
      "datasource": null,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 33
      },
      "id": 6,
      "panels": [],
      "title": "Chaincode Metrics",
      "type": "row"
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "chaincode_shim_requests_received",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 8,
        "w": 12,
        "x": 0,
        "y": 34
      },
      "hiddenSeries": false,
      "hideTimeOverride": false,
      "id": 15,
      "legend": {
        "alignAsTable": true,
        "avg": false,
        "current": false,
        "hideEmpty": true,
        "hideZero": true,
        "max": false,
        "min": false,
        "rightSide": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "null",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "rate(chaincode_shim_requests_received[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{channel}}, {{Received}}, {{chaincode}}, {{type}}",
          "refId": "A"
        },
        {
          "expr": "rate(chaincode_shim_requests_completed[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{channel}}, {{Completed}},  {{chaincode}}, {{type}}",
          "refId": "B"
        },
        {
          "expr": "histogram_quantile(0.95, sum(rate(chaincode_shim_request_duration_bucket[5m])) by (le))",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "",
          "refId": "C"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Chaincode Shim Requests",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "none",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "broadcast_enqueue_duration_sum",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 8,
        "w": 12,
        "x": 12,
        "y": 34
      },
      "hiddenSeries": false,
      "hideTimeOverride": false,
      "id": 16,
      "legend": {
        "alignAsTable": false,
        "avg": false,
        "current": false,
        "hideEmpty": false,
        "hideZero": true,
        "max": false,
        "min": false,
        "rightSide": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 2,
      "links": [],
      "nullPointMode": "connected",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "rate(go_memstats_frees_total{job!=\"prometheus\"}[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{channel}}, {{job}}",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Broadcast Enqueu",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "rate(grpc_comm_conn_closed[5m])",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 8,
        "w": 12,
        "x": 0,
        "y": 42
      },
      "hiddenSeries": false,
      "id": 28,
      "legend": {
        "alignAsTable": false,
        "avg": false,
        "current": false,
        "hideEmpty": true,
        "hideZero": true,
        "max": false,
        "min": false,
        "rightSide": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "null",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "rate(grpc_comm_conn_closed[5m])",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{job}}",
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "gRPC Closed Connections",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": null,
      "description": "chaincode_shim_requests_received",
      "fill": 1,
      "fillGradient": 0,
      "gridPos": {
        "h": 8,
        "w": 12,
        "x": 12,
        "y": 42
      },
      "hiddenSeries": false,
      "id": 14,
      "legend": {
        "alignAsTable": true,
        "avg": false,
        "current": false,
        "hideEmpty": true,
        "hideZero": true,
        "max": false,
        "min": false,
        "rightSide": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "null",
      "options": {
        "dataLinks": []
      },
      "paceLength": 10,
      "percentage": false,
      "pointradius": 2,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "chaincode_launch_duration_count",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{Count}}, {{chaincode}}",
          "refId": "B"
        },
        {
          "expr": "chaincode_launch_duration_sum",
          "format": "time_series",
          "intervalFactor": 1,
          "legendFormat": "{{Tota}}, {{chaincode}}",
          "refId": "C"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Chaincode Launch Duration",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "none",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": false
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    }
  ],
  "refresh": "5s",
  "schemaVersion": 21,
  "style": "dark",
  "tags": [],
  "templating": {
    "list": []
  },
  "time": {
    "from": "now-5m",
    "to": "now"
  },
  "timepicker": {
    "refresh_intervals": [
      "5s",
      "10s",
      "30s",
      "1m",
      "5m",
      "15m",
      "30m",
      "1h",
      "2h",
      "1d"
    ],
    "time_options": [
      "5m",
      "15m",
      "1h",
      "6h",
      "12h",
      "24h",
      "2d",
      "7d",
      "30d"
    ]
  },
  "timezone": "",
  "title": "Hyperledger Fabric Monitoring (Backend)",
  "uid": "monitor_fabric_backend",
  "version": 1,
  "license": "Apache-2.0"
}`

const DashboardFabricBusinessTemplate = `{
  "annotations": {
    "list": [
      {
        "builtIn": 1,
        "datasource": "-- Grafana --",
        "enable": true,
        "hide": true,
        "iconColor": "rgba(0, 211, 255, 1)",
        "name": "Annotations & Alerts",
        "type": "dashboard"
      }
    ]
  },
  "editable": true,
  "gnetId": null,
  "graphTooltip": 0,
  "id": 3,
  "links": [],
  "panels": [
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorValue": false,
      "colors": ["#d44a3a", "rgba(237, 129, 40, 0.89)", "#299c46"],
      "datasource": "Prometheus",
      "format": "none",
      "gauge": {
        "maxValue": 5,
        "minValue": 0,
        "show": true,
        "thresholdLabels": true,
        "thresholdMarkers": true
      },
      "gridPos": {
        "h": 4,
        "w": 3,
        "x": 0,
        "y": 0
      },
      "id": 22,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": true,
        "lineColor": "rgb(31, 120, 193)",
        "show": true
      },
      "tableColumn": "Value",
      "targets": [
        {
          "expr": "sum(rate(ledger_transaction_count[5m]))",
          "format": "time_series",
          "instant": true,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": "0,1",
      "title": "Transactions per second",
      "type": "singlestat",
      "valueFontSize": "80%",
      "valueMaps": [
        {
          "op": "=",
          "text": "0",
          "value": "null"
        },
        {
          "op": "=",
          "text": "0",
          "value": "N/A"
        }
      ],
      "valueName": "avg"
    },
    {
      "cacheTimeout": null,
      "colorBackground": false,
      "colorValue": false,
      "colors": ["#299c46", "rgba(237, 129, 40, 0.89)", "#d44a3a"],
      "datasource": "Prometheus",
      "format": "none",
      "gauge": {
        "maxValue": 20,
        "minValue": 0,
        "show": true,
        "thresholdLabels": true,
        "thresholdMarkers": true
      },
      "gridPos": {
        "h": 4,
        "w": 3,
        "x": 3,
        "y": 0
      },
      "id": 23,
      "interval": null,
      "links": [],
      "mappingType": 1,
      "mappingTypes": [
        {
          "name": "value to text",
          "value": 1
        },
        {
          "name": "range to text",
          "value": 2
        }
      ],
      "maxDataPoints": 100,
      "nullPointMode": "connected",
      "nullText": null,
      "postfix": "",
      "postfixFontSize": "50%",
      "prefix": "",
      "prefixFontSize": "50%",
      "rangeMaps": [
        {
          "from": "null",
          "text": "N/A",
          "to": "null"
        }
      ],
      "sparkline": {
        "fillColor": "rgba(31, 118, 189, 0.18)",
        "full": true,
        "lineColor": "rgb(31, 120, 193)",
        "show": true
      },
      "tableColumn": "Value",
      "targets": [
        {
          "expr": "sum(rate(chaincode_execute_timeouts[1m]))",
          "format": "time_series",
          "instant": false,
          "intervalFactor": 1,
          "legendFormat": "",
          "refId": "A"
        },
        {
          "expr": "sum(rate(chaincode_launch_failures[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "B"
        },
        {
          "expr": "sum(rate(chaincode_launch_timeouts[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "C"
        },
        {
          "expr": "sum(rate(endorser_chaincode_instantiation_failures[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "D"
        },
        {
          "expr": "sum(rate(endorser_duplicate_transaction_failures[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "E"
        },
        {
          "expr": "sum(rate(endorser_endorsement_failures[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "F"
        },
        {
          "expr": "sum(rate(endorser_proposal_acl_failures[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "G"
        },
        {
          "expr": "sum(rate(endorser_proposal_validation_failures[1h]))",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "H"
        }
      ],
      "thresholds": "0,0",
      "title": "Failures per second during  last minute",
      "type": "singlestat",
      "valueFontSize": "80%",
      "valueMaps": [
        {
          "op": "=",
          "text": "0",
          "value": "null"
        },
        {
          "op": "=",
          "text": "0",
          "value": "N/A"
        }
      ],
      "valueName": "total"
    },
    {
      "columns": [],
      "datasource": "Prometheus",
      "fontSize": "100%",
      "gridPos": {
        "h": 4,
        "w": 8,
        "x": 6,
        "y": 0
      },
      "id": 18,
      "links": [],
      "pageSize": null,
      "scroll": true,
      "showHeader": true,
      "sort": {
        "col": null,
        "desc": false
      },
      "styles": [
        {
          "alias": "Time",
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "pattern": "Time",
          "type": "date"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "channel",
          "thresholds": [],
          "type": "string",
          "unit": "short"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "instance",
          "thresholds": [],
          "type": "string",
          "unit": "short"
        },
        {
          "alias": "Blocks",
          "colorMode": null,
          "colors": [
            "rgba(50, 172, 45, 0.97)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(245, 54, 54, 0.9)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 0,
          "mappingType": 1,
          "pattern": "Value",
          "thresholds": ["0"],
          "type": "number",
          "unit": "none"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "__name__",
          "thresholds": [],
          "type": "hidden",
          "unit": "short"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "job",
          "thresholds": [],
          "type": "hidden",
          "unit": "short"
        }
      ],
      "targets": [
        {
          "expr": "ledger_blockchain_height",
          "format": "table",
          "instant": true,
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "title": "Ledger Height Per Channel",
      "transform": "table",
      "type": "table"
    },
    {
      "columns": [],
      "datasource": "Prometheus",
      "fontSize": "100%",
      "gridPos": {
        "h": 4,
        "w": 8,
        "x": 14,
        "y": 0
      },
      "hideTimeOverride": false,
      "id": 12,
      "interval": "1m",
      "links": [],
      "pageSize": null,
      "repeat": null,
      "scroll": true,
      "showHeader": true,
      "sort": {
        "col": null,
        "desc": false
      },
      "styles": [
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "Time",
          "thresholds": [],
          "type": "date",
          "unit": "short"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "__name__",
          "thresholds": [],
          "type": "hidden",
          "unit": "short"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "job",
          "thresholds": [],
          "type": "hidden",
          "unit": "short"
        },
        {
          "alias": "",
          "colorMode": null,
          "colors": [
            "rgba(245, 54, 54, 0.9)",
            "rgba(237, 129, 40, 0.89)",
            "rgba(50, 172, 45, 0.97)"
          ],
          "dateFormat": "YYYY-MM-DD HH:mm:ss",
          "decimals": 2,
          "mappingType": 1,
          "pattern": "Value",
          "thresholds": [],
          "type": "hidden",
          "unit": "short"
        }
      ],
      "targets": [
        {
          "expr": "fabric_version",
          "format": "table",
          "instant": true,
          "intervalFactor": 1,
          "legendFormat": "",
          "refId": "A"
        }
      ],
      "timeFrom": null,
      "timeShift": null,
      "title": "Live Monitored Nodes",
      "transform": "table",
      "transparent": false,
      "type": "table"
    },
    {
      "cards": {
        "cardPadding": null,
        "cardRound": null
      },
      "color": {
        "cardColor": "#b4ff00",
        "colorScale": "sqrt",
        "colorScheme": "interpolateSpectral",
        "exponent": 0.5,
        "max": null,
        "min": null,
        "mode": "spectrum"
      },
      "dataFormat": "tsbuckets",
      "datasource": "Prometheus",
      "gridPos": {
        "h": 9,
        "w": 12,
        "x": 0,
        "y": 4
      },
      "heatmap": {},
      "highlightCards": true,
      "id": 20,
      "legend": {
        "show": true
      },
      "links": [],
      "targets": [
        {
          "expr": "ledger_block_processing_time_bucket\n",
          "format": "heatmap",
          "instant": false,
          "interval": "",
          "intervalFactor": 10,
          "legendFormat": "{{le}}",
          "refId": "A"
        }
      ],
      "title": "Block procesing time",
      "tooltip": {
        "show": true,
        "showHistogram": false
      },
      "type": "heatmap",
      "xAxis": {
        "show": true
      },
      "xBucketNumber": null,
      "xBucketSize": "",
      "yAxis": {
        "decimals": null,
        "format": "short",
        "logBase": 1,
        "max": null,
        "min": null,
        "show": true,
        "splitFactor": null
      },
      "yBucketBound": "auto",
      "yBucketNumber": null,
      "yBucketSize": null
    },
    {
      "aliasColors": {},
      "bars": false,
      "dashLength": 10,
      "dashes": false,
      "datasource": "Prometheus",
      "fill": 1,
      "gridPos": {
        "h": 9,
        "w": 12,
        "x": 12,
        "y": 4
      },
      "id": 14,
      "legend": {
        "avg": false,
        "current": false,
        "max": false,
        "min": false,
        "show": true,
        "total": false,
        "values": false
      },
      "lines": true,
      "linewidth": 1,
      "links": [],
      "nullPointMode": "null",
      "percentage": false,
      "pointradius": 5,
      "points": false,
      "renderer": "flot",
      "seriesOverrides": [],
      "spaceLength": 10,
      "stack": false,
      "steppedLine": false,
      "targets": [
        {
          "expr": "sum(ledger_transaction_count{transaction_type=\"ENDORSER_TRANSACTION\"}) by (channel)",
          "format": "time_series",
          "intervalFactor": 1,
          "refId": "A"
        }
      ],
      "thresholds": [],
      "timeFrom": null,
      "timeRegions": [],
      "timeShift": null,
      "title": "Amount of business transactions per channel",
      "tooltip": {
        "shared": true,
        "sort": 0,
        "value_type": "individual"
      },
      "type": "graph",
      "xaxis": {
        "buckets": null,
        "mode": "time",
        "name": null,
        "show": true,
        "values": []
      },
      "yaxes": [
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        },
        {
          "format": "short",
          "label": null,
          "logBase": 1,
          "max": null,
          "min": null,
          "show": true
        }
      ],
      "yaxis": {
        "align": false,
        "alignLevel": null
      }
    }
  ],
  "refresh": "5s",
  "schemaVersion": 16,
  "style": "dark",
  "tags": [],
  "templating": {
    "list": []
  },
  "time": {
    "from": "now-5m",
    "to": "now"
  },
  "timepicker": {
    "refresh_intervals": [
      "5s",
      "10s",
      "30s",
      "1m",
      "5m",
      "15m",
      "30m",
      "1h",
      "2h",
      "1d"
    ],
    "time_options": ["5m", "15m", "1h", "6h", "12h", "24h", "2d", "7d", "30d"]
  },
  "timezone": "",
  "title": "Hyperledger Fabric Monitoring (Business)",
  "uid": "monitor_fabric_business",
  "version": 1,
  "license": "Apache-2.0"
}`
