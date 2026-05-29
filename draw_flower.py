import turtle

# 设置画布
turtle.setup(800, 600)

# 创建画笔
p = turtle.Pen()
p.speed(10)

# 画花
def draw_flower():
    for i in range(36):
        p.color('red')
        p.begin_fill()
        p.color('yellow')
        p.end_fill()
        p.circle(100, 60)
        p.left(120)
        p.circle(100, 60)
        p.left(120)
        p.left(10)

# 运行
draw_flower()
turtle.done()