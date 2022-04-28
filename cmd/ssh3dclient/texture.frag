#version 300 es

#ifdef GL_ES
precision highp float;
#endif

in vec2 texCoord;
in vec3 normal;
in vec3 lightVec;

out vec4 fragColor;

uniform sampler2D texSampler1;
uniform sampler2D texSampler2;

uniform float mixture;

uniform vec3 ambientCol; // The light and object's combined ambient color
uniform vec3 diffuseCol;  // The light and object's combined diffuse color

const float invRadiusSq = 0.00001;

void main() {
    // base color from diffuse texture
    vec4 col1 = texture(texSampler1, texCoord);
    vec4 col2 = texture(texSampler2, texCoord);

    vec4 col = mix(col1, col2, mixture);

    // ambient lighting
    vec3 ambient = vec3(ambientCol * col.xyz);

    // Calculate the light attenuation and direction.
    float distSq = dot(lightVec, lightVec);
    float attenuation = clamp(1.0 - invRadiusSq * sqrt(distSq), 0.0, 1.0);
    attenuation *= attenuation;
    vec3 lightDir = lightVec * inversesqrt(distSq);

    // Diffuse lighting
    vec3 diffuse = max(dot(lightDir, normal), 0.0) * diffuseCol * col.xyz;

    vec3 finalCol = (ambient + diffuse)*attenuation;

    fragColor = vec4(finalCol, col.w);
}
