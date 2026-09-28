import json

with open('organic-tests/reports/organic-test-summary.json', 'r') as f:
    data = json.load(f)

print(f"Total: {data['totals'].get('total', 0)} Passed: {data['totals'].get('passed', 0)} Failed: {data['totals'].get('failed', 0)} Skipped: {data['totals'].get('skipped', 0)} Blocked: {data['totals'].get('blocked', 0)}")

for r in data.get('results', []):
    if r['status'] != 'PASS':
        print(f"{r['id']}: {r['status']} - {r.get('triage', '')}")
