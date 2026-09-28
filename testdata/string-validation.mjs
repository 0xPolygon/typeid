import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const corpora = JSON.parse(readFileSync(0, 'utf8'));
let count = 0;
for (const { name, rules, cases } of corpora) {
  const pattern = new RegExp(`^(?:${rules.pattern})$`);
  // JavaScript's $ can match before a final newline, so check the full match.
  const isValid = (input) =>
    typeof input === 'string' && pattern.exec(input)?.[0] === input;
  for (const { input, valid } of cases) {
    assert.equal(isValid(input), valid, `${name}: ${JSON.stringify(input)}`);
    count++;
  }
  const validInput = cases.find(({ valid }) => valid).input;
  for (const input of [undefined, null, 0, true, {}, [], [validInput], new String(validInput), Symbol('id')]) {
    assert.equal(isValid(input), false, `${name}: must reject non-string input`);
  }
}
console.log(`${count} inputs matched UnmarshalText across ${corpora.length} ID types`);
