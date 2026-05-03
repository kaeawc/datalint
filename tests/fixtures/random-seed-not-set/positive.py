import random

data = [1, 2, 3, 4, 5]
random.shuffle(data)
train, eval = data[:3], data[3:]
