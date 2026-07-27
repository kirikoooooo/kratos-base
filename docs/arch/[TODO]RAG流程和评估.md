RAG 主流程

1. 入库阶段
1）根据文件类型进行分类（目前仅包含PDF，txt，md）
2）针对PDF格式的数据，采用MinerU，本地跑起来Pipeline模式进行解析
3）对解析后产生的资源文件夹进行处理，文本类数据按照trunk划分方式进行向量数据库入库，这里暂时采用父子索引的方式，根据md的标题格式分级，向量数据库只存储子片段，
父子id映射存入redis中，并且同时进行关键词索引的构建【用于后续召回BM25的计算】


2. 召回阶段
1）BM25进行过滤查询，过滤较低的片段
2）向量检索+混合加权，从向量数据库中找寻到匹配子片段
3）根据匹配子片段，从redis中查找匹配的父片段
4）将父亲片段拼接到上下文中


存储源选型：
demo阶段：
1. 向量数据库暂时使用faiss,存储子片段和对应id。
2. 父子关系映射采用redis，redis可以使用docker来进行构建。
3. BM25所需的倒排索引也存储在redis当中。
4. 【可选】向量数据库做成可扩展的接口，redis也可以作为向量数据库的存储源。
5. MinerU开源项目地址：https://github.com/opendatalab/MinerU/blob/master/README_zh-CN.md
6. ragas开源项目地址：https://github.com/vibrantlabsai/ragas


测试评估：
1. third_party 中引入ragas
2. 配置ragas 中的测评方式为llm as a judge
3. 如需使用apikey，请使用以下配置，并在config.yml 里增加对应配置
deepseek
[https://api.deepseek.com](https://api.deepseek.com/)
sk-ce707938242041618b9ad50d7514d8d6
4. 将ragas 接入评估工作流中去，cmd文件夹下新建入库，评估相关指令，cmd层代码里写好对应的源数据文件夹和output文件夹路径（包含解析后资源产物，以及评估结果表的内容）
