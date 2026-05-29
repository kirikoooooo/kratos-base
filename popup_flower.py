import tkinter as tk

# 创建主窗口
root = tk.Tk()
root.title('弹窗')
root.geometry('300x200')

# 创建画布
canvas = tk.Canvas(root, width=300, height=200)
canvas.pack()

# 画花
def draw_flower(x, y):
    # 花瓣
    for _ in range(6):
    # 这行代码是错误的，删除它
        canvas.create_oval(x-20, y-20, x+20, y+20, fill='pink')
        canvas.create_oval(x-20, y-20, x+20, y+20, fill='pink', outline='')
        canvas.rotate(60)
        canvas.create_oval(x-20, y-20, x+20, y+20, fill='pink', outline='')
    # 这行代码是错误的，删除它

    # 花心
    canvas.create_oval(x-10, y-10, x+10, y+10, fill='yellow')

# 画花
draw_flower(150, 100)

# 运行主循环
root.mainloop()