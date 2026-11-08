#!/usr/bin/env python3
"""Rehearse the PocketContext 0.24.x to Atlas upgrade runbook with synthetic data.

Pass --previous-binary for a verified 0.24.0 executable; otherwise the pinned
release archive is fetched and its checksum verified. Standard library only.
"""
import argparse
from contextlib import closing
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import tempfile
import time

import e2e
import upgrades

PREVIOUS = '0.24.0'


def settled(operation, *args):
    """Windows keeps files locked briefly after a stopped server exits."""
    for attempt in range(60):
        try:
            return operation(*args)
        except PermissionError:
            if attempt == 59:
                raise
            time.sleep(1)


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def rehearse(target, previous, root):
    home = root / 'home'
    home.mkdir()
    env = e2e.isolated_env(home)
    earlier_name = 'pocketcontext.exe' if os.name == 'nt' else 'pocketcontext'
    current_name = 'pocketcontext-atlas.exe' if os.name == 'nt' else 'pocketcontext-atlas'
    project = root / 'Project'
    project.mkdir()
    old = project / earlier_name
    new = project / current_name
    shutil.copy2(previous, old)

    def run(*args, binary=None, stdin=None, expect=0):
        result = subprocess.run([str(binary or old), '--db', str(project / 'pocketcontext.db'), *args],
                                input=stdin, capture_output=True, text=True, encoding='utf-8', env=env, timeout=120)
        if result.returncode != expect:
            raise AssertionError(f'{args}: exit {result.returncode}, expected {expect}\n{result.stdout}\n{result.stderr}')
        return result

    def data(*args, **kwargs):
        return json.loads(run(*args, **kwargs).stdout)

    def check(*args, expect=0, **kwargs):
        return json.loads(run('upgrade-check', *args, binary=target, expect=expect, **kwargs).stdout.splitlines()[0])

    def status(result, check_id):
        return next(c['status'] for c in result['checks'] if c['id'] == check_id)

    port = e2e.free_port()
    data('init', '--name', 'Rehearsal', '--port', str(port), '--password-stdin', stdin=e2e.PASSWORD + '\n')
    note = root / 'evidence.txt'
    note.write_text('Synthetic upgrade evidence: the lighthouse is green.\n', encoding='utf-8')
    source = data('ingest', str(note), '--title', 'Upgrade evidence')['source']
    instance = json.loads((project / 'pocketcontext.json').read_text(encoding='utf-8'))['instance']

    # A new binary never adopts a server of the earlier identity, but can stop it.
    refused = run('ensure-running', binary=target, expect=1)
    assert 'earlier' in refused.stderr and PREVIOUS in refused.stderr, refused.stderr
    inventory = check('--from', PREVIOUS, '--inventory')
    assert inventory['ready'] and status(inventory, 'server.stopped') == 'warn', inventory
    blocked = check('--from', PREVIOUS, expect=3)
    assert not blocked['ready'] and status(blocked, 'server.stopped') == 'block', blocked
    run('stop', binary=target)

    # Confirm flush, then the complete stopped backup with its matching binary.
    flushed = check('--from', PREVIOUS)
    assert flushed['ready'], flushed
    assert status(flushed, 'workingCopy') == 'pass' and status(flushed, 'database.format') == 'pass', flushed
    before = sha(project / 'pocketcontext.db')
    config_before = (project / 'pocketcontext.json').read_bytes()
    backup = root / 'Backup'
    shutil.copytree(project, backup)

    # Upgrade: install beside the old executable, move the old one away, then verify.
    shutil.copy2(target, new)
    settled(os.remove, old)
    run('ensure-running', binary=new)
    st = data('status', binary=new)
    assert st['running'] and st['instance'] == instance and st['port'] == port, st
    version = st['version']
    assert version not in ('', PREVIOUS), st
    run('skill', '--agent', 'claude-code', '--dir', str(home / 'skills' / 'atlas'), binary=new)
    assert (home / 'skills' / 'atlas' / '.pocketcontext-atlas').exists()
    rows = data('sql', "SELECT title FROM sources WHERE id = '" + source + "'", binary=new)['rows']
    assert rows == [['Upgrade evidence']], rows
    run('stop', binary=new)

    # Identity alone did not rewrite the configuration format or lose the data.
    config_after = json.loads((project / 'pocketcontext.json').read_text(encoding='utf-8'))
    assert config_after['instance'] == instance and config_after['port'] == port
    assert set(config_after) >= set(json.loads(config_before))

    # No migration added by the identity change: same applied ledger.
    def migrations(path):
        with closing(sqlite3.connect(path.as_uri() + '?mode=ro', uri=True)) as db:
            return sorted(row[0] for row in db.execute('SELECT file FROM _migrations'))

    assert migrations(project / 'pocketcontext.db') == migrations(backup / 'pocketcontext.db')

    # Rollback: restore the complete backup and matching binary, then verify.
    settled(shutil.rmtree, project)
    shutil.copytree(backup, project)
    assert sha(project / 'pocketcontext.db') == before
    assert subprocess.check_output([str(old), '--version'], text=True, env=env).split()[1] == PREVIOUS
    run('ensure-running')
    assert data('status')['instance'] == instance
    run('stop')
    print(f'PASS: PocketContext {PREVIOUS} -> Atlas {version}; not adopted, stopped by new binary, data and identity verified, rollback verified')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--previous-binary', type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix='pocketcontext-upgrades-', ignore_cleanup_errors=True) as directory:
        root = Path(directory)
        upgrades.PREVIOUS = PREVIOUS
        previous = args.previous_binary.resolve() if args.previous_binary else upgrades.download_previous(root / 'previous-release')
        rehearse(args.binary.resolve(), previous, root)


if __name__ == '__main__':
    main()
