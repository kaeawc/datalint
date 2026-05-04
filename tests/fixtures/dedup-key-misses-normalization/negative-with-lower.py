import pandas as pd

df = pd.read_csv("data.csv")
df["prompt"] = df["prompt"].str.lower().str.strip()
df = df.drop_duplicates(subset=["prompt"])
df.to_csv("out.csv", index=False)
