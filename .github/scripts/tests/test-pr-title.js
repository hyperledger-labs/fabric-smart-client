// SPDX-License-Identifier: Apache-2.0
//
// tests/test-pr-title.js
//
// Unit tests for helpers/pr-title.js and integration tests for bot-pr-title.js.
// Run with: node .github/scripts/tests/test-pr-title.js

const { runTestSuite, createMockGithub } = require('./test-utils');
const {
  parseTitle,
  labelFor,
  labelChanges,
  CHANGELOG_LABELS,
  TITLE_FORM,
} = require('../helpers/pr-title');
const script = require('../bot-pr-title');

// =============================================================================
// UNIT TESTS
// =============================================================================

/** Returns the label for a title, or 'INVALID' when the title does not parse. */
function label(title) {
  const parsed = parseTitle(title);
  return parsed.ok ? labelFor(parsed) : 'INVALID';
}

const unitTests = [
  // ---------------------------------------------------------------------------
  // labelFor: one label per type
  // ---------------------------------------------------------------------------
  { name: 'feat with scope → feature', test: () => label('feat(view): add x') === 'feature' },
  { name: 'feat without scope → feature', test: () => label('feat: add x') === 'feature' },
  { name: 'fix → fix', test: () => label('fix(fabric): resolve a txID') === 'fix' },
  { name: 'perf → perf', test: () => label('perf(storage): index wallet_id') === 'perf' },
  { name: 'refactor with ! → breaking', test: () => label('refactor(storage)!: replace squirrel') === 'breaking' },
  { name: 'fix! → breaking (breaking wins over type)', test: () => label('fix!: y') === 'breaking' },
  { name: 'feat with scope and ! → breaking', test: () => label('feat(view)!: authorize responders') === 'breaking' },
  { name: 'chore(deps) → dependencies', test: () => label('chore(deps): bump grpc') === 'dependencies' },
  { name: 'build(deps) → dependencies', test: () => label('build(deps): bump x') === 'dependencies' },
  { name: 'chore without deps scope → none', test: () => label('chore: tidy') === null },
  { name: 'chore(ci) → none', test: () => label('chore(ci): rename job') === null },
  { name: 'test → skip-changelog', test: () => label('test(integration): z') === 'skip-changelog' },
  { name: 'ci → skip-changelog', test: () => label('ci: z') === 'skip-changelog' },
  { name: 'build → skip-changelog', test: () => label('build: z') === 'skip-changelog' },
  { name: 'style → skip-changelog', test: () => label('style: z') === 'skip-changelog' },
  { name: 'refactor → none', test: () => label('refactor(view): split') === null },
  { name: 'docs → none', test: () => label('docs(dev): add a guide') === null },
  { name: 'revert type → none', test: () => label('revert: undo x') === null },
  { name: 'scope with slash, dot, dash → valid', test: () => label('fix(view/comm.ws-1): z') === 'fix' },

  // ---------------------------------------------------------------------------
  // git's own Revert header
  // ---------------------------------------------------------------------------
  {
    name: 'Revert "<title>" → valid, no label',
    test: () => {
      const p = parseTitle('Revert "feat(view): add x"');
      return p.ok === true && p.type === 'revert' && labelFor(p) === null;
    },
  },
  { name: 'Revert without quotes → invalid', test: () => parseTitle('Revert feat(view): add x').ok === false },

  // ---------------------------------------------------------------------------
  // invalid titles
  // ---------------------------------------------------------------------------
  { name: 'uppercase type → invalid', test: () => parseTitle('Feat: x').ok === false },
  { name: 'no space after colon → invalid', test: () => parseTitle('feat:x').ok === false },
  { name: 'unknown type → invalid', test: () => parseTitle('feature: x').ok === false },
  { name: 'missing colon → invalid', test: () => parseTitle('feat(view) add x').ok === false },
  { name: 'uppercase scope → invalid', test: () => parseTitle('feat(View): x').ok === false },
  { name: 'empty scope → invalid', test: () => parseTitle('feat(): x').ok === false },
  { name: 'empty summary → invalid', test: () => parseTitle('feat: ').ok === false },
  { name: 'empty title → invalid', test: () => parseTitle('').ok === false },
  { name: 'non-string title → invalid', test: () => parseTitle(undefined).ok === false },
  {
    name: 'error names the expected form',
    test: () => {
      const p = parseTitle('Feat: x');
      return p.ok === false && p.error.includes(TITLE_FORM) && p.error.includes('Feat: x');
    },
  },
  {
    name: 'error lists the allowed types',
    test: () => parseTitle('feature: x').error.includes('feat fix perf refactor docs test ci build chore style revert'),
  },

  // ---------------------------------------------------------------------------
  // labelChanges
  // ---------------------------------------------------------------------------
  {
    name: 'labelChanges: swaps feature for fix, keeps other labels',
    test: () => {
      const c = labelChanges(['feature', 'status: needs review'], 'fix');
      return JSON.stringify(c) === JSON.stringify({ add: ['fix'], remove: ['feature'] });
    },
  },
  {
    name: 'labelChanges: label already set → nothing to do',
    test: () => {
      const c = labelChanges(['fix', 'Fabric'], 'fix');
      return c.add.length === 0 && c.remove.length === 0;
    },
  },
  {
    name: 'labelChanges: wanted null removes every changelog label',
    test: () => {
      const c = labelChanges(['breaking', 'feature', 'EPIC'], null);
      return c.add.length === 0 && JSON.stringify(c.remove) === JSON.stringify(['breaking', 'feature']);
    },
  },
  {
    name: 'labelChanges: no labels yet → add only',
    test: () => JSON.stringify(labelChanges([], 'perf')) === JSON.stringify({ add: ['perf'], remove: [] }),
  },
  {
    name: 'CHANGELOG_LABELS lists the six labels',
    test: () =>
      JSON.stringify(CHANGELOG_LABELS) ===
      JSON.stringify(['breaking', 'feature', 'fix', 'perf', 'dependencies', 'skip-changelog']),
  },
];

// =============================================================================
// INTEGRATION TESTS (bot-pr-title.js against a mock GitHub)
// =============================================================================

function commit(message) {
  return { sha: 'abc123', commit: { message } };
}

/** Builds a GitHub mock that also records commit statuses. */
function mockGithub({ commits = [] } = {}) {
  const github = createMockGithub({ commits });
  github.calls.statuses = [];
  github.rest.repos = {
    createCommitStatus: async (params) => {
      github.calls.statuses.push({ state: params.state, context: params.context, description: params.description });
      return {};
    },
  };
  return github;
}

function context({ title, labels = [], commits = 1 }) {
  return {
    eventName: 'pull_request_target',
    repo: { owner: 'hyperledger-labs', repo: 'fabric-smart-client' },
    payload: {
      action: 'edited',
      pull_request: {
        number: 42,
        title,
        commits,
        head: { sha: 'headsha' },
        user: { login: 'alice' },
        labels: labels.map((name) => ({ name })),
      },
    },
  };
}

const scenarios = [
  {
    name: 'valid single-commit PR: success, label set',
    title: 'feat(view): add x',
    commits: [commit('feat(view): add x\n\nbody\n\nSigned-off-by: A <a@b.c>')],
    expect: { state: 'success', added: ['feature'], removed: [] },
  },
  {
    name: 'invalid title: failure naming the form, labels untouched',
    title: 'Feat: x',
    labels: ['feature'],
    commits: [commit('Feat: x')],
    expect: { state: 'failure', descriptionIncludes: '<type>(<scope>): <summary>', added: [], removed: [] },
  },
  {
    name: 'retitled feat → fix: swaps the label',
    title: 'fix(view): y',
    labels: ['feature', 'status: needs review'],
    commits: [commit('fix(view): y')],
    expect: { state: 'success', added: ['fix'], removed: ['feature'] },
  },
  {
    name: 'single commit whose header differs from the title: failure',
    title: 'fix(view): y',
    commits: [commit('fix(view): something else\n\nSigned-off-by: A <a@b.c>')],
    expect: { state: 'failure', descriptionIncludes: 'commit header', added: ['fix'], removed: [] },
  },
  {
    name: 'backport by Mergify: the cherry-picked header keeps the original PR number',
    title: 'fix(view): y (backport #2601)',
    commits: [commit('fix(view): y (#2601)\n\n(cherry picked from commit abc)')],
    expect: { state: 'success', added: ['fix'], removed: [] },
  },
  {
    name: 'backport of another PR: failure',
    title: 'fix(view): y (backport #2601)',
    commits: [commit('fix(view): y (#2602)')],
    expect: { state: 'failure', descriptionIncludes: 'commit header', added: ['fix'], removed: [] },
  },
  {
    name: 'several commits (fixups during review): header not compared',
    title: 'fix(view): y',
    prCommits: 2,
    commits: [commit('fix(view): y'), commit('fixup! fix(view): y')],
    expect: { state: 'success', added: ['fix'], removed: [] },
  },
  {
    name: 'refactor title: no label, stale changelog label removed',
    title: 'refactor(storage): split',
    labels: ['skip-changelog'],
    commits: [commit('refactor(storage): split')],
    expect: { state: 'success', added: [], removed: ['skip-changelog'] },
  },
];

async function runScenario(s, index) {
  console.log(`\n🧪 TEST ${index}: ${s.name}`);
  const github = mockGithub({ commits: s.commits });
  try {
    await script({ github, context: context({ title: s.title, labels: s.labels, commits: s.prCommits || s.commits.length }) });
  } catch (error) {
    console.log(`❌ threw: ${error.message}`);
    return false;
  }
  const status = github.calls.statuses[github.calls.statuses.length - 1];
  const checks = [
    ['one pr-title status', github.calls.statuses.length === 1 && status.context === 'pr-title'],
    [`state ${s.expect.state}`, status && status.state === s.expect.state],
    [
      `description includes "${s.expect.descriptionIncludes}"`,
      !s.expect.descriptionIncludes || (status && status.description.includes(s.expect.descriptionIncludes)),
    ],
    ['description fits GitHub limit', status && status.description.length <= 140],
    [`added ${JSON.stringify(s.expect.added)}`, JSON.stringify(github.calls.labelsAdded) === JSON.stringify(s.expect.added)],
    [
      `removed ${JSON.stringify(s.expect.removed)}`,
      JSON.stringify(github.calls.labelsRemoved) === JSON.stringify(s.expect.removed),
    ],
  ];
  let ok = true;
  for (const [what, passed] of checks) {
    console.log(`${passed ? '✅' : '❌'} ${what}`);
    if (!passed) ok = false;
  }
  return ok;
}

// =============================================================================
// TEST RUNNER
// =============================================================================

async function runUnitTests() {
  console.log('🔬 UNIT TESTS (pr-title)');
  console.log('='.repeat(70));
  let passed = 0;
  let failed = 0;
  for (const test of unitTests) {
    try {
      const result = await Promise.resolve(test.test());
      if (result) {
        console.log(`✅ ${test.name}`);
        passed++;
      } else {
        console.log(`❌ ${test.name}`);
        failed++;
      }
    } catch (error) {
      console.log(`❌ ${test.name} - Error: ${error.message}`);
      failed++;
    }
  }
  console.log('\n' + '-'.repeat(70));
  console.log(`Unit Tests: ${passed} passed, ${failed} failed`);
  return { total: unitTests.length, passed, failed };
}

runTestSuite('PR TITLE BOT TEST SUITE', scenarios, runScenario, [{ label: 'Unit Tests', run: runUnitTests }]);
