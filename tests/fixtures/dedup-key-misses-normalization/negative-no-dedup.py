import pandas as pd

df = pd.read_csv("data.csv")
df.to_csv("out.csv", index=False)
