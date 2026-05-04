def collect(rows):
    seen = set()
    for r in rows:
        if r["prompt"] not in seen:
            seen.add(r["prompt"])
            yield r


prompts = set(r["prompt"] for r in load())
