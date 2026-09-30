# 模拟模型 + 模拟 OpenAlex（端到端测试用，不需要真实 API Key）。用法：python3 fake_llm.py 11434
import json, re, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs
FR = re.compile(r'<fragment id="([A-Z]\d+)" source="([^"]*)">\n(.*?)\n</fragment>', re.S)
def answer(system, user):
    frags = FR.findall(user)
    if "检索助手" in system:
        return {"keywords_zh":["算法推荐","注意力","大学生"],"keywords_en":["algorithmic recommendation","attention span","college students"],
                "queries":[{"query":"algorithmic recommendation attention","lang":"en","note":"核心概念"},{"query":"短视频 算法推荐 注意力","lang":"zh","note":"用于知网"}],"tips":"先用英文检索式看综述。"}
    if "引用核查" in system:
        claim = re.search(r"<claim>(.*?)</claim>", user, re.S).group(1)
        cm = re.search(r"路程为(\d+)米", claim)
        for a, s, t in frags:
            m = re.search(r"路程为(\d+)米", t)
            if cm and m and m.group(1) == cm.group(1): return {"verdict": "支持", "cites": [a], "note": "数值一致"}
        if frags: return {"verdict": "与原文不符", "cites": [frags[0][0]], "note": "数值不同"}
        return {"verdict": "未找到对应", "cites": []}
    q = re.search(r"问题：(.*)", user).group(1) if "问题：" in user else ""
    if "作者" in q or "哪一天" in q:
        return {"claims":[],"missing":["资料中没有作者和日期"],"example":""}
    claims=[{"text":t,"type":"原文支持","cites":[a]} for a,s,t in frags if "路程" in t and re.search(r"\d",t)]
    return {"claims":claims,"missing":[] if claims else ["资料未涉及该问题"],"example":""}
def agent(sysm,msgs):
    folder=re.search(r"当前授权文件夹：(.+?)（",sysm)
    folder=folder.group(1) if folder else ""
    n=sum(1 for m in msgs if m["role"]=="assistant")
    last=msgs[-1]["content"]
    first=[m for m in msgs if m["role"]=="user" and not m["content"].startswith("【")][-1]["content"]
    # 只对最近一条用户消息之后的步骤计数
    idx=max(i for i,m in enumerate(msgs) if m["role"]=="user" and not m["content"].startswith("【"))
    n=sum(1 for m in msgs[idx:] if m["role"]=="assistant")
    if not folder: return {"reply":"请先授权文件夹。"}
    if n==0: return {"say":"先看看图片上的批注","tool":"recognize_image","args":{"image":"图片1","task":"marks"}} if "图片1" in first else {"tool":"list_folders","args":{}}
    if n==1: return {"say":"找一下论文的 tex 文件","tool":"search_files","args":{"path":folder,"name":"*.tex"}}
    if n==2:
        p=re.search(r"^(/.+?\.tex)",last,re.M)
        return {"tool":"read_file","args":{"path":p.group(1) if p else folder+"/main.tex"}}
    if n==3: return {"tool":"edit_file","args":{"path":folder+"/main.tex","old":"\\caption{表 1 实验结果}","new":"\\caption{表 1 主要实验结果}","reason":"按图片1 的批注修改表题"}}
    if n==4: return {"tool":"check_latex","args":{"path":folder+"/main.tex"}}
    return {"reply":"已按批注提交 1 处修改（表题改为“表 1 主要实验结果”），请在右侧确认后写入。LaTeX 检查没有发现问题。"}
def agent4(sysm,msgs):
    folder=re.search(r"当前授权文件夹：(.+?)（",sysm)
    folder=folder.group(1) if folder else ""
    idx=max(i for i,m in enumerate(msgs) if m["role"]=="user" and not m["content"].startswith("【"))
    first=msgs[idx]["content"]; last=msgs[-1]["content"]
    n=sum(1 for m in msgs[idx:] if m["role"]=="assistant")
    if not folder: return {"reply":"请先授权文件夹。"}
    if "Word" in first:
        doc=folder+"/report.docx"
        steps=[{"raw":'我先读一下文档。<｜DSML｜function_calls><｜DSML｜invoke name="read_file"><｜DSML｜parameter name="path" string="true">'+doc+'</｜DSML｜parameter></｜DSML｜invoke></｜DSML｜function_calls>'},
               {"say":"改摘要，并在引言后加一段","tool":"docx_edit","args":{"path":doc,"reason":"按要求改写摘要","edits":[{"op":"replace","index":1,"text":"摘要：本文用两周“无推荐流”干预实验，研究短视频使用时长与大学生注意力的关系。"},{"op":"insert_after","index":3,"text":"已有研究大多依赖自评问卷，缺少客观的使用记录。"}]}},
               {"say":"再生成一份修改说明","tool":"make_docx","args":{"path":folder+"/修改说明.docx","title":"修改说明","reason":"记录这次改了什么","content":"# 修改说明\n\n| 位置 | 修改 |\n|---|---|\n| 摘要 | 补充了研究方法 |\n| 引言 | 新增一段研究缺口 |\n\n<p style=\"text-align: right;\">CanDo 自动生成</p>"}},
               {"reply":"已提交 2 处 Word 修改（保留原有格式和图片）和一份修改说明，请在右侧确认。"}]
        return steps[min(n,3)]
    if "柱状图" in first:
        steps=[{"say":"用外部工具画图","tool":"ext.fig.bar_chart","args":{"values":[3,5,2],"labels":["甲","乙","丙"],"title":"三组对比","save_to":folder}},
               {"reply":"柱状图已生成，请在右侧确认保存。"}]
        return steps[min(n,1)]
    if "整理" in first:
        steps=[{"say":"先建一个文件夹放图片","tool":"file_op","args":{"op":"mkdir","to":folder+"/figs","reason":"放图片"}},
               {"tool":"file_op","args":{"op":"move","from":folder+"/plot.png","to":folder+"/figs/plot.png","reason":"图片归类"}},
               {"tool":"file_op","args":{"op":"delete","from":folder+"/main.aux","reason":"编译中间文件"}},
               {"reply":"已提出 3 项整理操作，请在右侧确认。"}]
        return steps[min(n,3)]
    steps=[{"tool":"list_folders","args":{}},
           {"say":"先用 Python 算一下","tool":"run_command","args":{"command":"python3 -c \"print(6*7)\"","cwd":folder}},
           {"say":"编译论文","tool":"compile_latex","args":{"path":folder+"/main.tex"}},
           {"tool":"remember","args":{"text":"论文在 "+folder+"，用 XeLaTeX 编译"}},
           {"reply":"计算结果 42；论文已编译成功，生成的 PDF 在右侧“待确认”里，可以预览或保存到文件夹。"}]
    return steps[min(n,4)]
SKETCH="""===意图===
一个三步流程图：输入 → 处理 → 输出，用箭头从左到右连接。
===TIKZ===
\\begin{tikzpicture}[node distance=2.2cm, box/.style={draw, rounded corners, minimum width=1.8cm, minimum height=0.9cm}]
\\node[box] (a) {输入};
\\node[box, right=of a] (b) {处理};
\\node[box, right=of b] (c) {输出};
\\draw[->, thick] (a) -- (b);
\\draw[->, thick] (b) -- (c);
\\end{tikzpicture}
===SVG===
<svg width="520" height="120" viewBox="0 0 520 120" onload="alert(1)"><rect x="0" y="0" width="520" height="120" fill="#fff"/><script>alert(1)</script>
<defs><marker id="ar" markerWidth="10" markerHeight="10" refX="9" refY="3" orient="auto"><path d="M0,0 L9,3 L0,6 z"/></marker></defs>
<rect x="20" y="35" width="110" height="50" rx="8" fill="none" stroke="#000"/><text x="75" y="66" text-anchor="middle">输入</text>
<rect x="205" y="35" width="110" height="50" rx="8" fill="none" stroke="#000"/><text x="260" y="66" text-anchor="middle">处理</text>
<rect x="390" y="35" width="110" height="50" rx="8" fill="none" stroke="#000"/><text x="445" y="66" text-anchor="middle">输出</text>
<line x1="130" y1="60" x2="203" y2="60" stroke="#000" marker-end="url(#ar)"/><line x1="315" y1="60" x2="388" y2="60" stroke="#000" marker-end="url(#ar)"/></svg>
===说明===
手绘的第三个框没有闭合，按方框处理。"""
WORKS=[{"id":"https://openalex.org/W1","doi":"https://doi.org/10.1000/abc.1","display_name":"Algorithmic recommendation and attention in short video use","publication_year":2023,
  "authorships":[{"author":{"display_name":"John A. Smith"}},{"author":{"display_name":"Li Wei"}}],"primary_location":{"source":{"display_name":"Computers in Human Behavior"},"landing_page_url":"https://example.org/w1"},
  "biblio":{"volume":"140","issue":"2","first_page":"107","last_page":"118"},"type":"article","cited_by_count":42,"open_access":{"is_oa":True,"oa_url":"PDFURL"},
  "best_oa_location":{"pdf_url":"PDFURL"},"abstract_inverted_index":{"Short":[0],"video":[1],"platforms":[2],"shape":[3],"attention.":[4]},"language":"en"},
 {"id":"https://openalex.org/W2","doi":None,"display_name":"短视频使用与大学生注意力研究","publication_year":2022,
  "authorships":[{"author":{"display_name":"张三"}},{"author":{"display_name":"李四"}}],"primary_location":{"source":{"display_name":"心理科学"},"landing_page_url":"https://example.org/w2"},
  "biblio":{"volume":"45","issue":"3","first_page":"20","last_page":"28"},"type":"article","cited_by_count":5,"open_access":{"is_oa":False},"abstract_inverted_index":None,"language":"zh"}]
PDF=open(sys.argv[2],"rb").read() if len(sys.argv)>2 else b"%PDF-1.4"
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        u=urlparse(self.path)
        if u.path=="/v1/models":
            body=json.dumps({"data":[{"id":"qwen2.5:7b"},{"id":"qwen2.5:32b"},{"id":"nomic-embed-text"}]}).encode()
            self.send_response(200); self.send_header("Content-Type","application/json"); self.end_headers(); self.wfile.write(body); return
        if u.path=="/cr/works":
            item={"DOI":"10.1000/cr.1","title":["Short video use and attention: a Crossref record"],"type":"journal-article",
                  "author":[{"given":"Mary","family":"Lee"}],"container-title":["Journal of Attention"],"issued":{"date-parts":[[2021]]},"is-referenced-by-count":12}
            body=json.dumps({"message":{"total-results":88,"items":[item]}}).encode()
            self.send_response(200); self.send_header("Content-Type","application/json"); self.end_headers(); self.wfile.write(body); return
        if u.path=="/works" and "api_key" not in parse_qs(u.query) and __import__("os").environ.get("OA_QUOTA"):
            self.send_response(429); self.send_header("X-RateLimit-Remaining","0"); self.send_header("X-RateLimit-Reset","7200"); self.end_headers()
            self.wfile.write(b'{"error":"Daily budget exceeded"}'); return
        if u.path=="/works":
            base=f"http://127.0.0.1:{self.server.server_port}"
            ws=json.loads(json.dumps(WORKS).replace("PDFURL",base+"/paper.pdf"))
            body=json.dumps({"meta":{"count":len(ws)},"results":ws}).encode(); ct="application/json"
            self.send_response(200); self.send_header("Content-Type",ct); self.send_header("X-RateLimit-Remaining","0.9"); self.send_header("X-RateLimit-Credits-Used","0.001"); self.end_headers(); self.wfile.write(body); return
        elif u.path=="/paper.pdf": body=PDF; ct="application/pdf"
        else: self.send_response(404); self.end_headers(); return
        self.send_response(200); self.send_header("Content-Type",ct); self.end_headers(); self.wfile.write(body)
    def do_POST(self):
        b=json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        msgs=b["messages"]; sysm=msgs[0]["content"]; user=msgs[1]["content"]
        if "OCR 文字识别助手" in sysm:
            out="第一章 匀速运动\n\n物体以恒定速度运动时，路程等于速度乘以时间，即 $s=vt$。\n\n例如速度为 10 米/秒，持续 5 秒，路程为 50 米。"
            body=json.dumps({"choices":[{"message":{"content":out}}],"usage":{"prompt_tokens":1500,"completion_tokens":120}}).encode()
        elif "科研绘图助手" in sysm:
            txt=user[0]["text"] if isinstance(user,list) else user
            out=SKETCH
            if "修改要求" in txt: out=SKETCH.replace("\\draw[->, thick]","\\draw[->, thick, dashed]").replace('stroke="#000" marker','stroke="#000" stroke-dasharray="6 4" marker')
            body=json.dumps({"choices":[{"message":{"content":out}}]}).encode()
        elif "写成一份“论文契约”" in sysm:
            __import__("time").sleep(2)
            fs=re.findall(r'<fragment id="(F\d+)"',user); ns=re.findall(r'^(N\d+)：',user,re.M)
            secs=re.search(r"这种论文的章节：(.*)",user).group(1).split("、")
            out={"question":{"text":"每日短视频使用时长是否会降低大学生的持续注意力？","variables":[{"name":"每日短视频使用时长","type":"independent","measure":"手机屏幕使用记录（分钟/天）"},{"name":"持续注意力","type":"dependent","measure":"注意力测验得分与专注时长"},{"name":"睡眠时长","type":"control","measure":"自报"}],
                 "method":"问卷与屏幕记录的相关分析 + 两周无推荐流干预实验","scope":"某校本科生 60 人；不研究中学生和其他平台"},
                 "claims":[{"text":"短视频使用时长增加会导致大学生持续注意力下降","novelty":"用客观的屏幕记录代替自评","evidence":ns[:1]+fs[:1],"falsify":"控制睡眠时长后两者不再相关"},
                           {"text":"关闭推荐流两周可以改善专注时长","novelty":"提出一个可以直接操作的干预","evidence":ns[-1:],"falsify":"干预组与对照组的专注时长没有差异"}],
                 "sections":[{"name":n,"question":"这一节回答："+n+"要说明的问题","evidence":(fs[:1] or ns[:1])[0] if (fs or ns) else "需补充","conclusion":n+"的结论","link":"承上启下"} for n in secs[:5]],
                 "missing":["对照组的基线数据"]}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":3000,"completion_tokens":700}}).encode()
        elif "站在审稿人的角度对它发起质疑" in sysm:
            out={"attacks":[{"kind":"confound","target":"K1","text":"睡眠不足可能同时导致刷短视频更多和注意力更差，你怎么排除？","severity":"high","hint":"控制睡眠时长后的分析结果"},
                            {"kind":"sample","target":"Q","text":"样本只来自一所学校，结论能推广到其他大学生吗？","severity":"medium","hint":"说明抽样方法，或把结论限定在本校"},
                            {"kind":"measurement","target":"K2","text":"专注时长是怎么测的？学生知道自己在被测，会不会故意表现更好？","severity":"medium","hint":"说明测量工具和是否盲测"}]}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":2500,"completion_tokens":400}}).encode()
        elif "判断这条回应能不能让你撤回质疑" in sysm:
            resp=re.search(r"<response>\n(.*?)\n</response>",user,re.S).group(1)
            ids=[x for x in re.findall(r"[NF]\d+",resp)]
            nums=[x.strip() for x in re.split(r"[。；;\n]",resp) if re.search(r"\d",x)]
            core=any(k in resp for k in ["随机","对照","控制","睡眠","抽样","盲"])
            if core and (nums or ids):
                out={"addresses_core":True,"evidence":ids+nums[:1],"evidence_ok":True,"verdict":"concede","reason":"回应针对质疑的核心，并给出了具体数据。","next":""}
            else:
                # 模拟“想讨好作者”的模型：没有证据也说让步（服务端规则应拦下）
                out={"addresses_core":core,"evidence":[],"evidence_ok":False,"verdict":"concede","reason":"作者态度坚决，可以接受。","next":"请给出具体数据"}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":1800,"completion_tokens":150}}).encode()
        elif "论证骨架”（先想清楚" in sysm:
            __import__("time").sleep(3)
            fs=re.findall(r'<fragment id="(F\d+)"',user); ns=re.findall(r'^(N\d+)：',user,re.M)
            out={"claims":[{"role":"background","text":"短视频平台依靠算法推荐延长用户使用时间","evidence":fs[:1]},
                           {"role":"gap","text":"频繁切换内容是否是注意力下降的原因仍不清楚","evidence":fs[1:2] or fs[:1]},
                           {"role":"design","text":"我们设计了两周的无推荐流干预实验","evidence":ns[:1]},
                           {"role":"finding","text":"干预后专注时长从 18 分钟增加到 24 分钟","evidence":ns[-1:]},
                           {"role":"boundary","text":"样本只来自一所学校","evidence":[]}],"missing":["对照组的基线数据"],"advice":"引言最后一段要和结果部分的顺序一致。"}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":3000,"completion_tokens":500}}).encode()
        elif "把论文的某一节写成" in sysm:
            fs=re.findall(r'<fragment id="(F\d+)"',user); ns=re.findall(r'^(N\d+)：',user,re.M)
            out={"paragraphs":[{"sentences":[{"text":"短视频平台普遍依靠算法推荐延长用户的使用时间。","cites":fs[:1],"kind":"claim"},
                                               {"text":"然而，频繁切换内容是否是大学生注意力下降的原因，目前仍缺少直接证据。","cites":["C2"],"kind":"claim"}]},
                               {"sentences":[{"text":"为此，本研究设计了为期两周的无推荐流干预实验。","cites":ns[:1],"kind":"claim"},
                                             {"text":"结果显示，干预组的专注时长显著增加。","cites":ns[-1:],"kind":"claim"},
                                             {"text":"这一发现具有重要意义。","cites":[],"kind":"claim"},
                                             {"text":"本研究的样本量为【需补充：两组样本量】。","cites":[],"kind":"gap"}]}],"notes":["请核对 24 分钟的统计口径"]}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":3500,"completion_tokens":600}}).encode()
        elif "逐句对照契约，找出写偏的地方" in sysm:
            sids=re.findall(r'^(S\d+)：',user,re.M)
            out={"items":[{"kind":"new_claim","sentence":sids[1] if len(sids)>1 else "","text":"契约里没有“频繁切换是原因”这一主张，这句把它当成了研究结论","fix":"改成“本研究将检验……”，或先更新契约"},
                          {"kind":"contradict","sentence":"S99","text":"（编造的句子编号，应被丢弃）","fix":""}],"covered":True}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":2600,"completion_tokens":200}}).encode()
        elif "筛选助手" in sysm:
            ids=re.findall(r'<paper id="(P\d+)">\n题名：(.*)',user)
            items=[{"id":i,"score":3 if "attention" in t.lower() or "注意力" in t else 1,"reason":"题名直接涉及注意力" if "attention" in t.lower() or "注意力" in t else "只部分相关","fields":{"研究对象":"大学生","方法":"问卷调查","样本/数据":"摘要未提及","主要结论":"短视频使用与注意力下降有关"}} for i,t in ids]
            body=json.dumps({"choices":[{"message":{"content":json.dumps({"items":items},ensure_ascii=False)}}],"usage":{"prompt_tokens":2000,"completion_tokens":400}}).encode()
        elif "科研调研助手" in sysm:
            ns=[int(x) for x in re.findall(r'<paper n="(\d+)">',user)]
            out={"summary":"现有研究多认为短视频使用与注意力下降相关，但因果证据不足。","sections":[{"heading":"主要发现","points":[{"text":"算法推荐的短视频使用与注意力持续时间缩短相关","cites":ns[:2]},{"text":"编造的结论","cites":[999]}]}],
                 "disagreements":[],"gaps":["缺少纵向和实验研究"],"next_steps":["设计一个两周的干预实验"],"next_queries":["short video attention longitudinal"] if "longitudinal" not in user else []}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}]}).encode()
        elif "带新同学写论文" in sysm:
            out={"sections":[{"name":"摘要","questions":["每个问题用了什么模型？","关键结果是多少？"],"subheadings":[],"materials":["各问题的结果数字"],"pitfall":"不要写成背景介绍"},
                             {"name":"问题重述","questions":["题目的核心要求是什么？"],"subheadings":["问题背景","需要解决的问题"],"materials":["题目原文"],"pitfall":"不要照抄题目"}],"next":["先整理数据","确定每个问题的模型","写摘要初稿"]}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}]}).encode()
        elif "指导学生写论文" in sysm:
            out={"items":[{"level":"warn","where":"摘要","problem":"摘要没有给出具体结果数字","suggestion":"写出调度成本降低的百分比"}],"overall":"结构完整，摘要需要更具体。"}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}]}).encode()
        elif "LaTeX 排版助手" in sysm:
            txt=user[0]["text"] if isinstance(user,list) else user
            has_img=isinstance(user,list) and any(p.get("type")=="image_url" for p in user)
            if not has_img: out="（没有收到图片）"
            elif "批注" in txt: out="1. 第 3 行表题：原文“表 1 实验结果” → 改为“表 1 主要实验结果”（红笔圈注）"
            elif "表格" in txt and "判断" not in txt: out="\\begin{table}[htbp]\n\\centering\n\\begin{tabular}{cc}\n\\toprule\na & b \\\\\n\\bottomrule\n\\end{tabular}\n\\end{table}"
            elif "转写为 LaTeX。只输出" in txt or "数学公式" in txt: out="\\frac{a^{2}+b^{2}}{2} = c^{2}"
            else: out="```latex\n由勾股定理 $a^2+b^2=c^2$，可得\n\\begin{equation}\n\\frac{a^{2}+b^{2}}{2} = c^{2}\n\\end{equation}\n```"
            body=json.dumps({"choices":[{"message":{"content":out}}]}).encode()
        elif "读懂学术文献" in sysm:
            frags=FR.findall(user)
            fid=frags[0][0] if frags else "F1"
            if "概念：" in user:
                out={"plain":"匀速运动是速度大小和方向都不变的运动。","formula":"s = v t","example":"以 2 米/秒走 3 秒，路程 6 米。",
                     "in_paper":[{"text":"文中假设车辆以 10 米/秒匀速运动","cites":[fid]}],"prerequisites":[{"concept":"速度","why":"匀速是指速度不变"}],
                     "learn_next":["匀加速运动","位移"],"confusions":"匀速不等于速率不变（方向也要不变）。"}
            else:
                out={"summary":"用匀速运动模型计算 5 秒内的路程。","question":{"text":"车辆 5 秒内走多远","cites":[fid]},
                     "method":{"text":"用 路程=速度×时间 计算","cites":[fid]},"findings":[{"text":"5 秒内路程为 50 米","cites":[fid]}],
                     "limits":[{"text":"只在速度恒定时成立","cites":[frags[-1][0] if frags else fid]}],
                     "terms":[{"term":"匀速运动","note":"速度不变的运动"},{"term":"路程","note":"走过的轨迹长度"}],
                     "prerequisites":[{"concept":"速度与时间的关系","why":"理解路程公式"}],"gaps":["摘录中没有实验数据"]}
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":1200,"completion_tokens":300}}).encode()
        elif "本机智能体" in sysm:
            out=agent4(sysm,msgs)
            content=out["raw"] if "raw" in out else json.dumps(out,ensure_ascii=False)
            body=json.dumps({"choices":[{"message":{"content":content}}]}).encode()
        elif "本机文件助手" in sysm:
            out=agent(sysm,msgs)
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}]}).encode()
        else:
            out={"ok":True} if "连通性" in sysm else answer(sysm,user)
            body=json.dumps({"choices":[{"message":{"content":json.dumps(out,ensure_ascii=False)}}],"usage":{"prompt_tokens":900,"completion_tokens":200}}).encode()
        self.send_response(200); self.send_header("Content-Type","application/json"); self.end_headers(); self.wfile.write(body)
    def log_message(self,*a): pass
__import__("http.server").server.ThreadingHTTPServer(("127.0.0.1",int(sys.argv[1])),H).serve_forever()
