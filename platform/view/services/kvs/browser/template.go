/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package browser

const TableTemplate = `
<!DOCTYPE html>
<html lang="en">
<body>
<table>
    <tr>
        <th>Key</th>
        <th>Value</th>
    </tr>
    {{ range Items}}
        <tr>
            <td>{{ .Key }}</td>
            <td>{{ .Value }}</td>
        </tr>
    {{ end}}
</table>
</body>
</html>
`
