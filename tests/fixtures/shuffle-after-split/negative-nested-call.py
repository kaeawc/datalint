import random
from sklearn.model_selection import train_test_split

random.seed(42)
data = [1, 2, 3, 4, 5]
# random.shuffle is the argument of train_test_split — it executes
# *before* the split, so the rule must not flag it.
train, test = train_test_split(random.shuffle(data) or data)
