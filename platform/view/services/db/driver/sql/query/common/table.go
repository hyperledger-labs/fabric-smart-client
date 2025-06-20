/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

type TableName string

// TODO: Support new alias per table in case a table is selected twice in the same query (for different joins)
func NewTable(name TableName) aliasedTable {
	return NewAliasedTable(name, TableAlias(name))
}

func NewAliasedTable(name TableName, alias TableAlias) aliasedTable {
	return aliasedTable{
		name:  name,
		alias: alias,
	}
}

type Table interface {
	JoinedTable
	Field(name FieldName) Field
}

type JoinedTable interface {
	ConditionSerializable
	Join(Table, ConditionSerializable) JoinedTable
}

type TableAlias string

type aliasedTable struct {
	name  TableName
	alias TableAlias
}

func (a aliasedTable) WriteString(_ CondInterpreter, sb Builder) {
	sb.WriteString(string(a.name))
	if len(a.alias) > 0 && string(a.alias) != string(a.name) {
		sb.WriteString(" AS ").WriteString(string(a.alias))
	}
}

func (a aliasedTable) Field(name FieldName) Field {
	return field{table: &a, name: name}
}

func (a aliasedTable) Alias() TableAlias { return a.alias }

func (a aliasedTable) Join(other Table, ons ConditionSerializable) JoinedTable {
	return joinedTable{
		tables:     []Table{a, other},
		conditions: []ConditionSerializable{ons},
	}
}

type joinedTable struct {
	tables     []Table
	conditions []ConditionSerializable
}

func (t joinedTable) Join(other Table, ons ConditionSerializable) JoinedTable {
	t.tables = append(t.tables, other)
	t.conditions = append(t.conditions, ons)
	return t
}

func (t joinedTable) WriteString(in CondInterpreter, sb Builder) {
	sb.WriteConditionSerializable(t.tables[0], in)
	for i, tt := range t.tables[1:] {
		sb.WriteString(" LEFT JOIN ").
			WriteConditionSerializable(tt, in).
			WriteString(" ON ").
			WriteConditionSerializable(t.conditions[i], in)
	}
}
