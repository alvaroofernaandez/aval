import math, sys
H, S, ASC, GAP, CW = 100.0, 22.0, 150.0, 30.0, 112.0
def word(base=ASC):
    prims=[]; x=0.0
    def A(x):
        r=(H-S)/2; cx=x+S/2+r; cy=base-H/2; st=cx+r
        prims.append(('c',cx,cy,r)); prims.append(('s',st,base-H+S/2,st,base-S/2)); return H
    def V(x):
        p0=(x+S/2,base-H+S/2); p1=(x+CW*0.42,base-S/2); p2=(x+CW-S/2,base-ASC+S/2)
        prims.append(('s',*p0,*p1)); prims.append(('s',*p1,*p2)); return CW
    def L(x):
        cx=x+S/2; prims.append(('s',cx,base-ASC+S/2,cx,base-S/2)); return S
    for g,gap in ((A,GAP),(V,GAP*0.55),(A,GAP),(L,GAP)):
        x+=g(x)+gap
    return prims, x-GAP
def mark():
    cx,cy,r=184,314,112; tx,ty=440,86; jx,jy=cx+r+40,cy+r
    return [('c',cx,cy,r),('s',cx+r,cy,jx,jy),('s',jx,jy,tx,ty)], 64.0
def segd(px,py,x1,y1,x2,y2):
    dx,dy=x2-x1,y2-y1; L2=dx*dx+dy*dy
    t=0 if L2==0 else max(0,min(1,((px-x1)*dx+(py-y1)*dy)/L2))
    return math.hypot(px-(x1+t*dx),py-(y1+t*dy))
def inside(prims,sw,px,py):
    for p in prims:
        if p[0]=='c':
            if abs(math.hypot(px-p[1],py-p[2])-p[3])<=sw/2: return True
        else:
            if segd(px,py,*p[1:])<=sw/2: return True
    return False
def grid(prims,sw,x0,y0,x1,y1,cols,rows,sx,sy,ss=4,th=0.42):
    # cols x rows pixels; each pixel covered by ss*ss samples
    g=[[0]*cols for _ in range(rows)]
    for j in range(rows):
        for i in range(cols):
            c=0
            for a in range(ss):
                for b in range(ss):
                    px=x0+(i+(a+.5)/ss)*(x1-x0)/cols; py=y0+(j+(b+.5)/ss)*(y1-y0)/rows
                    c+=inside(prims,sw,px,py)
            g[j][i]=1 if c/(ss*ss)>=th else 0
    return g
QUAD={(0,0,0,0):' ',(1,0,0,0):'▘',(0,1,0,0):'▝',(0,0,1,0):'▖',(0,0,0,1):'▗',(1,1,0,0):'▀',(0,0,1,1):'▄',(1,0,1,0):'▌',(0,1,0,1):'▐',(1,0,0,1):'▚',(0,1,1,0):'▞',(1,1,1,0):'▛',(1,1,0,1):'▜',(1,0,1,1):'▙',(0,1,1,1):'▟',(1,1,1,1):'█'}
def render(kind,rows_cells,which='word'):
    if which=='word': prims,w=word(); sw=S; x0,y0,x1,y1=-S/2,-0.0,w+S/2,ASC+0.0
    else: prims,sw=mark(); x0,y0,x1,y1=40,40,480,470
    aspect=(x1-x0)/(y1-y0)
    if kind=='half': px_r, cw, ch = rows_cells*2, 1, 2
    elif kind=='quad': px_r, cw, ch = rows_cells*2, 2, 2
    elif kind=='braille': px_r, cw, ch = rows_cells*4, 2, 4
    cols_cells=round(rows_cells*aspect*2.05)
    g=grid(prims,sw,x0,y0,x1,y1,cols_cells*cw,px_r,0,0)
    out=[]
    for r in range(rows_cells):
        line=''
        for c in range(cols_cells):
            if kind=='half':
                t,b=g[2*r][c],g[2*r+1][c]; line+={(0,0):' ',(1,0):'▀',(0,1):'▄',(1,1):'█'}[(t,b)]
            elif kind=='quad':
                k=(g[2*r][2*c],g[2*r][2*c+1],g[2*r+1][2*c],g[2*r+1][2*c+1]); line+=QUAD[k]
            else:
                bits=[(0,0,0x1),(1,0,0x2),(2,0,0x4),(0,1,0x8),(1,1,0x10),(2,1,0x20),(3,0,0x40),(3,1,0x80)]
                v=0
                for dy,dx,m in bits:
                    if g[4*r+dy][2*c+dx]: v|=m
                line+=chr(0x2800+v) if v else ' '
        out.append(line.rstrip())
    return '\n'.join(out)
for kind,rows,which in [(a,int(b),c) for a,b,c in (x.split(':') for x in sys.argv[1:])]:
    print(f'== {kind} {rows} rows {which}'); print(render(kind,rows,which)); print()
