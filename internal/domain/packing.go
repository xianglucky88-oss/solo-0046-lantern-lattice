package domain

func pixel(g Grid, x, y, z, face int) (int, int) { return x + face*g.Size[0], y + z*g.Size[1] }
