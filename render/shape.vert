#version 300 es

layout(location = 0) in vec3 vertPos;
layout(location = 1) in vec3 vertNormal;

out vec3 normal;
out vec3 lightVec;
out float fogDepth;

uniform mat4 mvMat;
uniform mat4 normalMat;
uniform mat4 projMat;
uniform vec3 lightPos; // position in view space

void main() {

   vec4 vp = vec4(vertPos, 1.0);

    // position in view space
    vec4 viewPos = mvMat * vp;

    // position
    gl_Position = projMat * viewPos;

    normal = normalize((normalMat * vec4(vertNormal, 1.0)).xyz);

    lightVec = lightPos - viewPos.xyz;

    fogDepth = -(mvMat * vp).z;
}
