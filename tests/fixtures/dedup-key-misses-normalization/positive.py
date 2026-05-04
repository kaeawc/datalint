import pandas as pd

df = pd.read_csv("data.csv")
df = df.drop_duplicates(subset=["prompt"])
df.to_csv("out.csv", index=False)
