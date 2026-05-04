"""Intentionally buggy pipeline.

Triggers:
- random-seed-not-set on both random.shuffle calls (no random.seed in file)
- shuffle-after-split on the second random.shuffle (after train_test_split)
- dedup-key-misses-normalization on drop_duplicates (no .lower/.strip/normalize anywhere)
"""

import pandas as pd
import random
from sklearn.model_selection import train_test_split

data = [1, 2, 3, 4, 5]
random.shuffle(data)
train, test = train_test_split(data)
random.shuffle(data)

df = pd.read_csv("data.csv")
df = df.drop_duplicates(subset=["prompt"])
df.to_csv("out.csv", index=False)
