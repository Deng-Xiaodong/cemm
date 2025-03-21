import random

def generate_multi_map(filename, num_keys, min_values, max_values):
    with open(filename, 'w') as file:
        for i in range(1, num_keys + 1):
            key = f"key{i}"
            num_values = random.randint(min_values, max_values)
            values = [f"value{i}_{j+1}" for j in range(num_values)]
            line = f"{key} {' '.join(values)}\n"
            file.write(line)

# 参数设置
filename = "data/multi_map.txt"
num_keys = 1000
min_values = 10
max_values = 50

# 生成文件
generate_multi_map(filename, num_keys, min_values, max_values)
print(f"文件 {filename} 已生成，包含 {num_keys} 个关键字，每个关键字的值数量在 {min_values} 到 {max_values} 之间。")