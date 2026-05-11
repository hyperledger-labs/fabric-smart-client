// SPDX-License-Identifier: Apache-2.0
//
// tests/test-on-comment-bot.js
//
// Unit tests for parseComment in bot-on-comment.js.
// Run with: node .github/scripts/tests/test-on-comment-bot.js 

const { runTestSuite } = require('./test-utils');
const { parseComment } = require('../bot-on-comment');

function deepEqual(a, b) {
  return JSON.stringify(a) === JSON.stringify(b);
}

const unitTests = [
  {
    name: 'exact assign',
    test: () => deepEqual(parseComment('/assign'), { commands: ['assign'] }),
  },
  {
    name: 'near miss assign',
    test: () => deepEqual(parseComment('/assign hi'), { nearMiss: 'assign' }),
  },
  {
    name: 'exact unassign',
    test: () => deepEqual(parseComment('/unassign'), { commands: ['unassign'] }),
  },
  {
    name: 'near miss unassign',
    test: () => deepEqual(parseComment('/unassign please'), { nearMiss: 'unassign' }),
  },
  {
    name: 'exact finalize',
    test: () => deepEqual(parseComment('/finalize'), { commands: ['finalize'] }),
  },
  {
    name: 'near miss finalize',
    test: () => deepEqual(parseComment('/finalize now'), { nearMiss: 'finalize' }),
  },
  {
    name: 'unrelated comment',
    test: () => deepEqual(parseComment('hello'), { commands: [] }),
  },
  {
    name: 'empty string',
    test: () => deepEqual(parseComment(''), { commands: [] }),
  },
];

async function runUnitTests() {
  console.log('🧪 UNIT TESTS (parseComment)');
  console.log('-'.repeat(50));

  let passed = 0;
  let failed = 0;

  for (const t of unitTests) {
    try {
      const result = await Promise.resolve(t.test());
      if (result) {
        console.log(`✅ ${t.name}`);
        passed++;
      } else {
        console.log(`❌ ${t.name}`);
        failed++;
      }
    } catch (error) {
      console.log(`❌ ${t.name} - Error: ${error.message}`);
      failed++;
    }
  }

  console.log('\n' + '-'.repeat(50));
  console.log(`Unit tests: ${passed} passed, ${failed} failed`);

  return { total: unitTests.length, passed, failed };
}

runTestSuite('ON-COMMENT BOT TEST SUITE', [], async () => true, [
  {
    label: 'Unit Tests',
    run: runUnitTests,
  },
]);