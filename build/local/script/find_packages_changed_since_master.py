#!/usr/bin/env python3

import os
import subprocess

mergebase = subprocess.run(
    'git merge-base origin/master HEAD'.split(),
    stdout=subprocess.PIPE,
    check=True,
).stdout.decode('utf8')

cmd = 'git diff --name-status {}'.format(mergebase)

statuses = subprocess.run(
    cmd.split(),
    stdout=subprocess.PIPE,
    check=True,
).stdout.decode('utf8')

files = []
for line in statuses.split('\n'):
    line = line.strip()
    if line == '':
        continue
    filename = line.split()[-1]
    if filename.startswith('vendor'):
        continue
    if filename.endswith('.go'):
        files.append(line.split()[-1])

packages = set()
for filename in files:
    dirname = os.path.dirname(filename)
    if not dirname:
        continue
    packages.add(dirname)

print(" ".join(["{}/...".format(p) for p in packages]))
