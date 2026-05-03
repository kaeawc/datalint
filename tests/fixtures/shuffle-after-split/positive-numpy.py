import numpy as np
from sklearn.model_selection import train_test_split

np.random.seed(42)
data = list(range(100))
train, test = train_test_split(data)
np.random.permutation(data)
