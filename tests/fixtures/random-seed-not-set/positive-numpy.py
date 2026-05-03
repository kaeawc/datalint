import numpy as np

data = list(range(100))
shuffled = np.random.permutation(data)
sample = np.random.choice(data, size=10)
