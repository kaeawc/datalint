import json


def load(path):
    with open(path) as f:
        return [json.loads(line) for line in f]


print(load("data.jsonl"))
