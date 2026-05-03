import random
from sklearn.model_selection import train_test_split

random.seed(42)
data = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
random.shuffle(data)
train, test = train_test_split(data, test_size=0.2)
