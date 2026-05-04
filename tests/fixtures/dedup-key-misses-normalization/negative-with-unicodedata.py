import unicodedata


def normalize(s):
    return unicodedata.normalize("NFKC", s)


def dedup(rows):
    return list({normalize(r["prompt"]): r for r in rows}.values())
